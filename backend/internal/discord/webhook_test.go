package discord

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shelfarr/internal/metadata"
)

// fakeDiscord starts an httptest server that accepts webhook POSTs and stores
// the last received message content for inspection.
func fakeDiscord(t *testing.T) (srv *httptest.Server, getContent func() string) {
	t.Helper()
	var last string
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p webhookPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		last = p.Content
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, func() string { return last }
}

// ── Send ──────────────────────────────────────────────────────────────────────

func TestSend_EmptyURL(t *testing.T) {
	// Empty webhook URL must be a complete no-op (no error, no network call).
	if err := Send(context.Background(), "", "hello"); err != nil {
		t.Fatalf("Send with empty URL: %v", err)
	}
}

func TestSend_DeliversContent(t *testing.T) {
	srv, getContent := fakeDiscord(t)

	if err := Send(context.Background(), srv.URL, "test message"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got := getContent(); got != "test message" {
		t.Errorf("content=%q; want %q", got, "test message")
	}
}

func TestSend_ContentTypeIsJSON(t *testing.T) {
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	_ = Send(context.Background(), srv.URL, "ping")
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Errorf("Content-Type=%q; want application/json", gotCT)
	}
}

func TestSend_NonSuccessStatusReturnsError(t *testing.T) {
	for _, code := range []int{http.StatusBadRequest, http.StatusTooManyRequests, http.StatusBadGateway} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "error", code)
			}))
			t.Cleanup(srv.Close)

			err := Send(context.Background(), srv.URL, "msg")
			if err == nil {
				t.Fatalf("expected error for HTTP %d", code)
			}
		})
	}
}

func TestSend_Unreachable(t *testing.T) {
	// Port 1 is never open; should fail with a connection error.
	err := Send(context.Background(), "http://127.0.0.1:1", "msg")
	if err == nil {
		t.Fatal("expected error when webhook URL is unreachable")
	}
}

// ── NotifyComplete ─────────────────────────────────────────────────────────────

func TestNotifyComplete_ContainsKeyFields(t *testing.T) {
	srv, getContent := fakeDiscord(t)

	book := &metadata.Book{
		Title:  "The Final Empire",
		Author: "Brandon Sanderson",
		Year:   2006,
	}
	err := NotifyComplete(context.Background(), srv.URL, book, "alice",
		"/audiobooks/Brandon Sanderson/The Final Empire (2006)")
	if err != nil {
		t.Fatalf("NotifyComplete: %v", err)
	}

	content := getContent()
	for _, want := range []string{
		"The Final Empire",
		"Brandon Sanderson",
		"alice",
		"/audiobooks/Brandon Sanderson/The Final Empire (2006)",
		"📚",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("message missing %q\ngot: %s", want, content)
		}
	}
}

func TestNotifyComplete_NoNarrator_OmitsNarratorLine(t *testing.T) {
	srv, getContent := fakeDiscord(t)

	book := &metadata.Book{Title: "Dune", Author: "Frank Herbert"} // no narrator
	_ = NotifyComplete(context.Background(), srv.URL, book, "bob", "/lib/Dune")

	if strings.Contains(getContent(), "Narrator") {
		t.Error("message should not contain 'Narrator' when book.Narrator is empty")
	}
}

func TestNotifyComplete_EmptyURL_NoOp(t *testing.T) {
	book := &metadata.Book{Title: "Any", Author: "Author"}
	if err := NotifyComplete(context.Background(), "", book, "user", "/path"); err != nil {
		t.Fatalf("expected no error with empty URL: %v", err)
	}
}

// ── NotifyFailed ──────────────────────────────────────────────────────────────

func TestNotifyFailed_ContainsKeyFields(t *testing.T) {
	srv, getContent := fakeDiscord(t)

	err := NotifyFailed(context.Background(), srv.URL,
		"Mistborn", "Brandon Sanderson", "charlie", "qBit torrent stalled after 2 hours")
	if err != nil {
		t.Fatalf("NotifyFailed: %v", err)
	}

	content := getContent()
	for _, want := range []string{
		"Mistborn",
		"Brandon Sanderson",
		"charlie",
		"qBit torrent stalled after 2 hours",
		"❌",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("message missing %q\ngot: %s", want, content)
		}
	}
}

func TestNotifyFailed_EmptyURL_NoOp(t *testing.T) {
	if err := NotifyFailed(context.Background(), "", "T", "A", "u", "reason"); err != nil {
		t.Fatalf("expected no error with empty URL: %v", err)
	}
}
