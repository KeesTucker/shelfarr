package qbit

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// ── parseMagnetHash ───────────────────────────────────────────────────────────

func TestParseMagnetHash(t *testing.T) {
	tests := []struct {
		name   string
		uri    string
		want   string
		wantOk bool
	}{
		{
			name:   "hex 40-char hash",
			uri:    "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&dn=Test",
			want:   "aabbccddeeff00112233445566778899aabbccdd",
			wantOk: true,
		},
		{
			name:   "uppercase hex normalised to lowercase",
			uri:    "magnet:?xt=urn:btih:AABBCCDDEEFF00112233445566778899AABBCCDD",
			want:   "aabbccddeeff00112233445566778899aabbccdd",
			wantOk: true,
		},
		{
			// 32 × 'A' base32 (no padding) = 20 zero bytes = 40 hex zeros.
			name:   "base32 32-char hash converted to hex",
			uri:    "magnet:?xt=urn:btih:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			want:   "0000000000000000000000000000000000000000",
			wantOk: true,
		},
		{
			name:   "no xt parameter",
			uri:    "magnet:?dn=NoHash",
			wantOk: false,
		},
		{
			name:   "wrong xt scheme",
			uri:    "magnet:?xt=urn:sha1:abc",
			wantOk: false,
		},
		{
			name:   "invalid URI",
			uri:    "://bad",
			wantOk: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseMagnetHash(tc.uri)
			if ok != tc.wantOk {
				t.Fatalf("ok=%v want %v (hash=%q)", ok, tc.wantOk, got)
			}
			if tc.want != "" && got != tc.want {
				t.Errorf("hash=%q want %q", got, tc.want)
			}
		})
	}
}

// ── fake qBit server helper ───────────────────────────────────────────────────

// fakeQBit starts an httptest.Server that simulates qBittorrent's Web API.
// infoResponse is returned as JSON for every GET /api/v2/torrents/info call.
// All non-login paths require a valid "SID" cookie.
func fakeQBit(t *testing.T, infoResponse []TorrentInfo) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/auth/login" {
			if _, err := r.Cookie("SID"); err != nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
		}
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/add":
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(infoResponse)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// ── AddTorrent ────────────────────────────────────────────────────────────────

func TestAddTorrentMagnet(t *testing.T) {
	const (
		magnet       = "magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd&dn=TestBook"
		expectedHash = "aabbccddeeff00112233445566778899aabbccdd"
	)
	srv := fakeQBit(t, nil)

	c := New(srv.URL, "admin", "pass")
	hash, err := c.AddTorrent(context.Background(), magnet, "/downloads", "")
	if err != nil {
		t.Fatalf("AddTorrent: %v", err)
	}
	if hash != expectedHash {
		t.Errorf("hash=%q want %q", hash, expectedHash)
	}
}

func TestAddTorrentQBitDown(t *testing.T) {
	c := New("http://127.0.0.1:1", "admin", "pass")
	_, err := c.AddTorrent(context.Background(),
		"magnet:?xt=urn:btih:aabbccddeeff00112233445566778899aabbccdd", "/dl", "")
	if err == nil {
		t.Fatal("expected error when qBit is unreachable")
	}
}

func TestAddTorrentNotConfigured(t *testing.T) {
	c := New("", "admin", "pass")
	_, err := c.AddTorrent(context.Background(), "magnet:?xt=urn:btih:abc", "/dl", "")
	if err == nil {
		t.Fatal("expected error when QBIT_URL is empty")
	}
}

// ── GetTorrent ────────────────────────────────────────────────────────────────

func TestGetTorrentFound(t *testing.T) {
	want := TorrentInfo{Hash: "deadbeef", Name: "Test Audiobook", Progress: 0.5}
	srv := fakeQBit(t, []TorrentInfo{want})

	c := New(srv.URL, "admin", "pass")
	got, err := c.GetTorrent(context.Background(), "deadbeef")
	if err != nil {
		t.Fatalf("GetTorrent: %v", err)
	}
	if got.Hash != want.Hash || got.Name != want.Name || got.Progress != want.Progress {
		t.Errorf("got %+v want %+v", got, want)
	}
}

func TestGetTorrentNotFound(t *testing.T) {
	srv := fakeQBit(t, nil) // server returns empty array
	c := New(srv.URL, "admin", "pass")
	_, err := c.GetTorrent(context.Background(), "nope")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetTorrentNotConfigured(t *testing.T) {
	c := New("", "admin", "pass")
	_, err := c.GetTorrent(context.Background(), "hash")
	if err == nil {
		t.Fatal("expected error when QBIT_URL is empty")
	}
}

// ── SetCategory ───────────────────────────────────────────────────────────────

func TestSetCategoryHappyPath(t *testing.T) {
	var gotHash, gotCategory string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/setCategory":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			gotHash = r.FormValue("hashes")
			gotCategory = r.FormValue("category")
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "admin", "pass")
	if err := c.SetCategory(context.Background(), "deadbeef", "imported"); err != nil {
		t.Fatalf("SetCategory: %v", err)
	}
	if gotHash != "deadbeef" {
		t.Errorf("hashes form field = %q; want %q", gotHash, "deadbeef")
	}
	if gotCategory != "imported" {
		t.Errorf("category form field = %q; want %q", gotCategory, "imported")
	}
}

func TestSetCategoryNotConfigured(t *testing.T) {
	c := New("", "admin", "pass")
	if err := c.SetCategory(context.Background(), "deadbeef", "imported"); err == nil {
		t.Fatal("expected error when QBIT_URL is empty")
	}
}

func TestSetCategoryServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/setCategory":
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "admin", "pass")
	if err := c.SetCategory(context.Background(), "deadbeef", "category"); err == nil {
		t.Fatal("expected error on non-200 response")
	}
}

func TestSetCategoryAutoCreateOnConflict(t *testing.T) {
	var createCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "test-sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/setCategory":
			if !createCalled {
				// First call: category doesn't exist yet.
				w.WriteHeader(http.StatusConflict)
				return
			}
			// Second call (after auto-create): succeed.
			w.WriteHeader(http.StatusOK)
		case "/api/v2/torrents/createCategory":
			createCalled = true
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "admin", "pass")
	if err := c.SetCategory(context.Background(), "deadbeef", "new-category"); err != nil {
		t.Fatalf("SetCategory: %v", err)
	}
	if !createCalled {
		t.Error("expected createCategory to be called on 409")
	}
}

// ── session re-login ──────────────────────────────────────────────────────────

// TestSessionReloginOn403 verifies that when qBittorrent returns 403 (session
// expired), the client re-authenticates and retries the request exactly once.
func TestSessionReloginOn403(t *testing.T) {
	var loginCount, infoCount int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			atomic.AddInt32(&loginCount, 1)
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "new-sid"})
			fmt.Fprint(w, "Ok.")
		case "/api/v2/torrents/info":
			n := atomic.AddInt32(&infoCount, 1)
			if n == 1 {
				// Simulate an expired session on the first call.
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode([]TorrentInfo{{Hash: "abc"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "admin", "pass")
	// Pre-populate with a stale cookie so ensureLoggedIn skips initial login.
	c.cookie = &http.Cookie{Name: "SID", Value: "expired-sid"}

	got, err := c.GetTorrent(context.Background(), "abc")
	if err != nil {
		t.Fatalf("GetTorrent: %v", err)
	}
	if got.Hash != "abc" {
		t.Errorf("unexpected hash: %q", got.Hash)
	}
	if n := atomic.LoadInt32(&loginCount); n != 1 {
		t.Errorf("expected 1 re-login call, got %d", n)
	}
	if n := atomic.LoadInt32(&infoCount); n != 2 {
		t.Errorf("expected 2 info calls (first 403, then retry), got %d", n)
	}
}
