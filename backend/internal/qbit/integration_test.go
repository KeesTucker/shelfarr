//go:build integration

package qbit_test

// Integration tests that talk to a real qBittorrent instance.
//
// Run with:
//
//	QBIT_USERNAME=admin QBIT_PASSWORD=... go test -tags integration -v ./internal/qbit/
//
// Optional overrides:
//
//	QBIT_URL=http://...    (default: http://10.10.10.2:8080)
//	QBIT_SAVE_PATH=...     (default: /downloads/audiobooks)

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"shelfarr/internal/qbit"
)

func integrationClient(t *testing.T) *qbit.Client {
	t.Helper()
	username := os.Getenv("QBIT_USERNAME")
	password := os.Getenv("QBIT_PASSWORD")
	if username == "" || password == "" {
		t.Skip("QBIT_USERNAME or QBIT_PASSWORD not set; skipping integration test")
	}
	baseURL := os.Getenv("QBIT_URL")
	if baseURL == "" {
		baseURL = "http://10.10.10.2:8080"
	}
	return qbit.New(baseURL, username, password)
}

func savePath() string {
	if p := os.Getenv("QBIT_SAVE_PATH"); p != "" {
		return p
	}
	return "/downloads/audiobooks"
}

// ── client ────────────────────────────────────────────────────────────────────

// TestIntegration_GetTorrentNotFound verifies that a nonexistent hash returns
// ErrNotFound (and that login succeeds implicitly).
func TestIntegration_GetTorrentNotFound(t *testing.T) {
	c := integrationClient(t)

	const fakeHash = "0000000000000000000000000000000000000000"
	_, err := c.GetTorrent(context.Background(), fakeHash)
	if !errors.Is(err, qbit.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for unknown hash, got %v", err)
	}
}

// TestIntegration_AddAndGetTorrent adds a magnet URI, retrieves the torrent
// info, and logs the result. The torrent is removed from qBittorrent on cleanup.
func TestIntegration_AddAndGetTorrent(t *testing.T) {
	c := integrationClient(t)

	// A well-known all-zero magnet that qBit will accept but never connect to.
	const magnet = "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&dn=shelfarr-integration-test"
	const wantHash = "aabbccddeeff00112233445566778899aabbccdd"

	hash, err := c.AddTorrent(context.Background(), magnet, savePath(), "")
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	t.Cleanup(func() {
		if err := c.RemoveTorrent(context.Background(), hash); err != nil {
			t.Logf("cleanup: RemoveTorrent: %v", err)
		}
	})
	if hash != wantHash {
		t.Errorf("hash=%q want %q", hash, wantHash)
	}

	// qBit registers magnet torrents asynchronously, so poll briefly.
	var info *qbit.TorrentInfo
	for range 5 {
		time.Sleep(time.Second)
		info, err = c.GetTorrent(context.Background(), hash)
		if err == nil {
			break
		}
		if !errors.Is(err, qbit.ErrNotFound) {
			t.Fatalf("GetTorrent after add: %v", err)
		}
	}
	if err != nil {
		t.Fatalf("torrent not visible after 5 retries: %v", err)
	}
	if info.Hash != hash {
		t.Errorf("info.Hash=%q want %q", info.Hash, hash)
	}
	t.Logf("torrent: name=%q state=%q progress=%.2f savePath=%q",
		info.Name, info.State, info.Progress, info.SavePath)
}

// TestIntegration_SetCategory adds a torrent, sets its category via
// SetCategory, and verifies qBittorrent accepts the call without error.
func TestIntegration_SetCategory(t *testing.T) {
	c := integrationClient(t)
	ctx := context.Background()

	const magnet = "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&dn=shelfarr-integration-test"
	const hash = "aabbccddeeff00112233445566778899aabbccdd"
	const category = "shelfarr-imported"

	// Add the torrent (idempotent — qBit silently ignores duplicate adds).
	if _, err := c.AddTorrent(ctx, magnet, savePath(), ""); err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	t.Cleanup(func() {
		if err := c.RemoveTorrent(ctx, hash); err != nil {
			t.Logf("cleanup: RemoveTorrent: %v", err)
		}
	})

	// Wait for the torrent to become visible.
	var err error
	for range 5 {
		time.Sleep(time.Second)
		_, err = c.GetTorrent(ctx, hash)
		if err == nil {
			break
		}
		if !errors.Is(err, qbit.ErrNotFound) {
			t.Fatalf("GetTorrent: %v", err)
		}
	}
	if err != nil {
		t.Fatalf("torrent not visible after 5 retries: %v", err)
	}

	if err := c.SetCategory(ctx, hash, category); err != nil {
		t.Fatalf("SetCategory: %v", err)
	}
	t.Logf("SetCategory(%q, %q) succeeded", hash, category)
}
