package qbit

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"bookarr/internal/db"
)

// ── fake torrent getter ───────────────────────────────────────────────────────

// fakeTorrentGetter is a thread-safe mock for the torrentGetter interface.
type fakeTorrentGetter struct {
	mu       sync.Mutex
	torrents map[string]*TorrentInfo
	err      error // if set, returned for every call
}

func (f *fakeTorrentGetter) GetTorrent(_ context.Context, hash string) (*TorrentInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	t, ok := f.torrents[hash]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (f *fakeTorrentGetter) set(hash string, info *TorrentInfo) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.torrents == nil {
		f.torrents = make(map[string]*TorrentInfo)
	}
	f.torrents[hash] = info
}

// ── test helpers ──────────────────────────────────────────────────────────────

func openWatcherTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// seedDownloading inserts a user + request in "downloading" status and returns
// the request. The torrent_hash is set to hash.
func seedDownloading(t *testing.T, d *db.DB, reqID, hash string) *db.Request {
	t.Helper()
	ctx := context.Background()
	_ = d.CreateUser(ctx, "u1", "alice", "pw", "user") // ok to duplicate across sub-tests
	req := &db.Request{
		ID:          reqID,
		UserID:      "u1",
		Title:       "Mistborn",
		Author:      "Sanderson",
		SearchQuery: "Mistborn Sanderson",
		TorrentHash: sql.NullString{String: hash, Valid: true},
		Status:      db.StatusDownloading,
	}
	if err := d.CreateRequest(ctx, req); err != nil {
		t.Fatalf("create request: %v", err)
	}
	return req
}

// newTestWatcher returns a Watcher with the fake getter wired in.
// The launch function is set to synchronous so that tests can assert on DB
// state immediately after w.tick() returns, without goroutine races.
func newTestWatcher(d *db.DB, fake *fakeTorrentGetter, onComplete OnComplete) *Watcher {
	w := NewWatcher(d, nil, onComplete, nil) // NewWatcher accepts *Client; we override below
	w.client = fake
	w.stallTimeout = time.Hour        // use explicit value; tests override as needed
	w.launch = func(f func()) { f() } // synchronous so tests can check state immediately
	return w
}

// ── tests ─────────────────────────────────────────────────────────────────────

// TestWatcherHappyPath: torrent downloads to 100% and enters an upload state
// → request transitions to done and onComplete is called exactly once.
func TestWatcherHappyPath(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef01"

	seedDownloading(t, d, "r1", hash)

	fake := &fakeTorrentGetter{}
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 1.0, State: "uploading"})

	var completionCalls int
	w := newTestWatcher(d, fake, func(_ context.Context, req *db.Request, _ *TorrentInfo) error {
		completionCalls++
		return nil
	})

	watched := make(map[string]*watchEntry)
	w.tick(ctx, watched)

	got, err := d.GetRequest(ctx, "r1")
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if got.Status != db.StatusDone {
		t.Errorf("status = %s; want done", got.Status)
	}
	if completionCalls != 1 {
		t.Errorf("onComplete called %d times; want 1", completionCalls)
	}
}

// TestWatcherProgressAcrossTicks: watcher correctly tracks partial progress
// across multiple ticks before completing.
func TestWatcherProgressAcrossTicks(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef02"

	seedDownloading(t, d, "r1", hash)

	fake := &fakeTorrentGetter{}
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 0.5, State: "downloading"})

	w := newTestWatcher(d, fake, nil)
	watched := make(map[string]*watchEntry)

	// First tick: 50% — should still be downloading.
	w.tick(ctx, watched)
	got, _ := d.GetRequest(ctx, "r1")
	if got.Status != db.StatusDownloading {
		t.Fatalf("after 50%%: status = %s; want downloading", got.Status)
	}

	// Second tick: 100% + uploading — should complete.
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 1.0, State: "uploading"})
	w.tick(ctx, watched)
	got, _ = d.GetRequest(ctx, "r1")
	if got.Status != db.StatusDone {
		t.Errorf("after 100%%: status = %s; want done", got.Status)
	}
}

// TestWatcherQBitErrorState: when qBit reports an error state the request is
// marked failed immediately.
func TestWatcherQBitErrorState(t *testing.T) {
	for _, state := range []string{"error", "missingFiles", "unknown"} {
		t.Run(state, func(t *testing.T) {
			d := openWatcherTestDB(t)
			ctx := context.Background()
			const hash = "deadbeef03"

			seedDownloading(t, d, "r1", hash)

			fake := &fakeTorrentGetter{}
			fake.set(hash, &TorrentInfo{Hash: hash, Progress: 0.3, State: state})

			w := newTestWatcher(d, fake, nil)
			watched := make(map[string]*watchEntry)
			w.tick(ctx, watched)

			got, _ := d.GetRequest(ctx, "r1")
			if got.Status != db.StatusFailed {
				t.Errorf("state %q: status = %s; want failed", state, got.Status)
			}
			if !got.Error.Valid || got.Error.String == "" {
				t.Errorf("state %q: expected non-empty error message", state)
			}
		})
	}
}

// TestWatcherStallDetection: when a torrent makes no progress for longer than
// stallTimeout the request is marked failed.
func TestWatcherStallDetection(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef04"

	seedDownloading(t, d, "r1", hash)

	fake := &fakeTorrentGetter{}
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 0.3, State: "downloading"})

	w := newTestWatcher(d, fake, nil)
	w.stallTimeout = 100 * time.Millisecond

	watched := make(map[string]*watchEntry)

	// First tick registers the entry.
	w.tick(ctx, watched)
	got, _ := d.GetRequest(ctx, "r1")
	if got.Status != db.StatusDownloading {
		t.Fatalf("before stall: status = %s; want downloading", got.Status)
	}

	// Wind back lastChange to simulate the stall timeout having elapsed.
	entry := watched[hash]
	entry.lastChange = time.Now().Add(-200 * time.Millisecond)

	// Second tick: stall detected.
	w.tick(ctx, watched)
	got, _ = d.GetRequest(ctx, "r1")
	if got.Status != db.StatusFailed {
		t.Errorf("after stall: status = %s; want failed", got.Status)
	}
	if !got.Error.Valid || got.Error.String != "qBit torrent stalled after 2 hours" {
		t.Errorf("unexpected error message: %q", got.Error.String)
	}
}

// TestWatcherOnCompleteError: when the onComplete callback returns an error
// the request is marked failed with that error message.
func TestWatcherOnCompleteError(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef05"

	seedDownloading(t, d, "r1", hash)

	fake := &fakeTorrentGetter{}
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 1.0, State: "stalledUP"})

	w := newTestWatcher(d, fake, func(_ context.Context, _ *db.Request, _ *TorrentInfo) error {
		return errors.New("disk full")
	})

	watched := make(map[string]*watchEntry)
	w.tick(ctx, watched)

	got, _ := d.GetRequest(ctx, "r1")
	if got.Status != db.StatusFailed {
		t.Errorf("status = %s; want failed", got.Status)
	}
	if !got.Error.Valid || got.Error.String != "disk full" {
		t.Errorf("error message = %q; want %q", got.Error.String, "disk full")
	}
}

// TestWatcherNotYetVisibleInQBit: ErrNotFound from qBit is silently ignored so
// the request stays downloading (the torrent may still be loading).
func TestWatcherNotYetVisibleInQBit(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef06"

	seedDownloading(t, d, "r1", hash)

	// Getter returns ErrNotFound (empty map).
	fake := &fakeTorrentGetter{}

	w := newTestWatcher(d, fake, nil)
	watched := make(map[string]*watchEntry)
	w.tick(ctx, watched)

	got, _ := d.GetRequest(ctx, "r1")
	if got.Status != db.StatusDownloading {
		t.Errorf("status = %s; want downloading (should wait quietly)", got.Status)
	}
}

// TestWatcherCheckingUpNotTriggered: progress=1.0 but state="checkingUP" must
// NOT trigger completion — qBit still holds an IO lock on the files.
func TestWatcherCheckingUpNotTriggered(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef07"

	seedDownloading(t, d, "r1", hash)

	fake := &fakeTorrentGetter{}
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 1.0, State: "checkingUP"})

	var completionCalls int
	w := newTestWatcher(d, fake, func(_ context.Context, _ *db.Request, _ *TorrentInfo) error {
		completionCalls++
		return nil
	})

	watched := make(map[string]*watchEntry)
	w.tick(ctx, watched)

	got, _ := d.GetRequest(ctx, "r1")
	if got.Status != db.StatusDownloading {
		t.Errorf("checkingUP: status = %s; want downloading", got.Status)
	}
	if completionCalls != 0 {
		t.Errorf("onComplete must not be called while qBit is checkingUP")
	}

	// Once qBit finishes checking and transitions to uploading, complete normally.
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 1.0, State: "uploading"})
	w.tick(ctx, watched)

	got, _ = d.GetRequest(ctx, "r1")
	if got.Status != db.StatusDone {
		t.Errorf("after checkingUP → uploading: status = %s; want done", got.Status)
	}
}

// TestWatcherStartupResume: a request already in "downloading" status when the
// server restarts is picked up on the first tick.
func TestWatcherStartupResume(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef08"

	seedDownloading(t, d, "r1", hash)
	fake := &fakeTorrentGetter{}
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 1.0, State: "uploading"})

	w := newTestWatcher(d, fake, nil)

	// The watched map starts empty, simulating a fresh goroutine after restart.
	watched := make(map[string]*watchEntry)
	w.tick(ctx, watched)

	got, _ := d.GetRequest(ctx, "r1")
	if got.Status != db.StatusDone {
		t.Errorf("after restart resume: status = %s; want done", got.Status)
	}
}

// TestWatcherPrunesResolvedEntry: if a request is resolved externally (e.g.
// manually set to failed/done), the watcher stops tracking its hash.
func TestWatcherPrunesResolvedEntry(t *testing.T) {
	d := openWatcherTestDB(t)
	ctx := context.Background()
	const hash = "deadbeef09"

	seedDownloading(t, d, "r1", hash)
	fake := &fakeTorrentGetter{}
	fake.set(hash, &TorrentInfo{Hash: hash, Progress: 0.5, State: "downloading"})

	w := newTestWatcher(d, fake, nil)
	watched := make(map[string]*watchEntry)

	// First tick: entry registered.
	w.tick(ctx, watched)
	if _, ok := watched[hash]; !ok {
		t.Fatal("expected hash to be tracked after first tick")
	}

	// Resolve externally.
	_ = d.UpdateRequestStatus(ctx, "r1", db.StatusFailed, db.WithError("manual"))

	// Second tick: entry should be pruned.
	w.tick(ctx, watched)
	if _, ok := watched[hash]; ok {
		t.Error("hash should have been pruned after request was resolved externally")
	}
}

// TestReadyToMove verifies the state whitelist used to guard file moves.
func TestReadyToMove(t *testing.T) {
	ready := []string{"uploading", "stalledUP", "pausedUP", "queuedUP", "forcedUP"}
	for _, s := range ready {
		if !readyToMove(s) {
			t.Errorf("readyToMove(%q) = false; want true", s)
		}
	}

	notReady := []string{"checkingUP", "moving", "downloading", "checkingDL", "error", ""}
	for _, s := range notReady {
		if readyToMove(s) {
			t.Errorf("readyToMove(%q) = true; want false", s)
		}
	}
}
