package qbit

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"bookarr/internal/db"
)

const (
	defaultWatchInterval = 30 * time.Second
	defaultStallTimeout  = 2 * time.Hour
)

// OnComplete is called when a torrent finishes downloading (progress ≥ 1.0).
// It runs with the request already in status "moving". Returning a non-nil
// error causes the request to be marked as failed with that error message.
// Steps 6 and 7 (metadata, file move, ABS rescan, Discord) are wired here.
type OnComplete func(ctx context.Context, req *db.Request, info *TorrentInfo) error

// torrentGetter is the subset of *Client used by the watcher. The narrow
// interface allows tests to substitute a lightweight fake.
type torrentGetter interface {
	GetTorrent(ctx context.Context, hash string) (*TorrentInfo, error)
}

// Watcher polls qBittorrent every interval for active download progress and
// transitions request statuses in the database. It is safe for concurrent use
// and runs as a daemon goroutine that automatically recovers from panics.
type Watcher struct {
	db           *db.DB
	client       torrentGetter
	onComplete   OnComplete
	interval     time.Duration
	stallTimeout time.Duration
}

// NewWatcher creates a Watcher. Pass nil for onComplete to simply mark
// completed downloads as done with no additional processing.
func NewWatcher(database *db.DB, client *Client, onComplete OnComplete) *Watcher {
	if onComplete == nil {
		onComplete = func(_ context.Context, _ *db.Request, _ *TorrentInfo) error {
			return nil
		}
	}
	return &Watcher{
		db:           database,
		client:       client,
		onComplete:   onComplete,
		interval:     defaultWatchInterval,
		stallTimeout: defaultStallTimeout,
	}
}

// Start launches the background polling goroutine and returns immediately.
// The goroutine runs until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) {
	go w.run(ctx)
}

// ── internal ──────────────────────────────────────────────────────────────────

type watchEntry struct {
	requestID    string
	lastProgress float64
	lastChange   time.Time // when progress was last seen to increase
}

func (w *Watcher) run(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("watcher: panic recovered — restarting", "panic", r)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				go w.run(ctx)
			}
		}
	}()

	watched := make(map[string]*watchEntry) // hash → entry
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	// Tick immediately so existing downloads are picked up on startup.
	w.tick(ctx, watched)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.tick(ctx, watched)
		}
	}
}

func (w *Watcher) tick(ctx context.Context, watched map[string]*watchEntry) {
	active, err := w.db.ListActiveDownloads(ctx)
	if err != nil {
		slog.Error("watcher: list active downloads", "err", err)
		return
	}

	// Register any newly submitted downloads not yet tracked.
	activeHashes := make(map[string]bool, len(active))
	for _, req := range active {
		if !req.TorrentHash.Valid {
			continue
		}
		hash := req.TorrentHash.String
		activeHashes[hash] = true
		if _, ok := watched[hash]; !ok {
			watched[hash] = &watchEntry{
				requestID:    req.ID,
				lastProgress: 0,
				lastChange:   req.UpdatedAt, // survives server restarts
			}
			slog.Info("watcher: tracking torrent", "request_id", req.ID, "hash", hash)
		}
	}

	// Prune entries no longer in the "downloading" state (e.g. manually resolved).
	for hash := range watched {
		if !activeHashes[hash] {
			delete(watched, hash)
		}
	}

	for hash, entry := range watched {
		w.checkTorrent(ctx, hash, entry, watched)
	}
}

func (w *Watcher) checkTorrent(ctx context.Context, hash string, entry *watchEntry, watched map[string]*watchEntry) {
	info, err := w.client.GetTorrent(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			// Torrent not yet visible in qBit (e.g. still loading) — skip silently.
			return
		}
		slog.Error("watcher: poll torrent", "hash", hash, "err", err)
		return
	}

	// Fail on qBit error states.
	switch info.State {
	case "error", "missingFiles", "unknown":
		w.markFailed(ctx, entry.requestID, hash,
			"qBit torrent entered error state: "+info.State, watched)
		return
	}

	// Update stall-detection timestamp whenever progress increases.
	if info.Progress > entry.lastProgress {
		entry.lastProgress = info.Progress
		entry.lastChange = time.Now()
	}

	// Stall detection: no progress for longer than stallTimeout.
	if info.Progress < 1.0 && time.Since(entry.lastChange) > w.stallTimeout {
		w.markFailed(ctx, entry.requestID, hash,
			"qBit torrent stalled after 2 hours", watched)
		return
	}

	// Only move files once qBit is in an upload/seeding state. When progress
	// first hits 1.0, qBit may still be in "checkingUP" (re-verifying file
	// integrity) or "moving" (relocating files itself). During those states it
	// holds an IO lock on the files, so attempting a move would fail or produce
	// a partial copy. We skip the tick and try again in the next interval.
	if info.Progress >= 1.0 && readyToMove(info.State) {
		w.handleComplete(ctx, entry.requestID, hash, info, watched)
	}
}

// readyToMove reports whether qBit's state indicates files are fully written
// and stable. Progress can reach 1.0 during "checkingUP" (qBit verifying
// integrity) or "moving" (qBit relocating files) — both hold IO locks.
func readyToMove(state string) bool {
	switch state {
	case "uploading", "stalledUP", "pausedUP", "queuedUP", "forcedUP":
		return true
	default:
		return false
	}
}

func (w *Watcher) handleComplete(ctx context.Context, requestID, hash string, info *TorrentInfo, watched map[string]*watchEntry) {
	slog.Info("watcher: download complete, beginning move", "request_id", requestID)

	if err := w.db.UpdateRequestStatus(ctx, requestID, db.StatusMoving); err != nil {
		slog.Error("watcher: set status moving", "request_id", requestID, "err", err)
		return
	}
	delete(watched, hash) // stop tracking; status is no longer "downloading"

	req, err := w.db.GetRequest(ctx, requestID)
	if err != nil {
		slog.Error("watcher: reload request after completion", "request_id", requestID, "err", err)
		_ = w.db.UpdateRequestStatus(ctx, requestID, db.StatusFailed,
			db.WithError("internal error: could not reload request"))
		return
	}

	if err := w.onComplete(ctx, req, info); err != nil {
		slog.Error("watcher: completion handler failed", "request_id", requestID, "err", err)
		_ = w.db.UpdateRequestStatus(ctx, requestID, db.StatusFailed,
			db.WithError(err.Error()))
		return
	}

	if err := w.db.UpdateRequestStatus(ctx, requestID, db.StatusDone); err != nil {
		slog.Error("watcher: set status done", "request_id", requestID, "err", err)
		return
	}
	slog.Info("watcher: request done", "request_id", requestID)
}

func (w *Watcher) markFailed(ctx context.Context, requestID, hash, reason string, watched map[string]*watchEntry) {
	slog.Error("watcher: marking request failed", "request_id", requestID, "hash", hash, "reason", reason)
	delete(watched, hash)
	if err := w.db.UpdateRequestStatus(ctx, requestID, db.StatusFailed, db.WithError(reason)); err != nil {
		slog.Error("watcher: update failed status", "request_id", requestID, "err", err)
	}
}
