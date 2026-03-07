package library

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandlerList_EmptyLibrary(t *testing.T) {
	h := NewHandler(t.TempDir())
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/library", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var entries []BookEntry
	if err := json.NewDecoder(rr.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(entries))
	}
}

func TestHandlerList_WithBooks(t *testing.T) {
	libDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(libDir, "Brandon Sanderson", "The Final Empire"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(libDir)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/library", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var entries []BookEntry
	if err := json.NewDecoder(rr.Body).Decode(&entries); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].AuthorFolder != "Brandon Sanderson" {
		t.Errorf("AuthorFolder=%q; want %q", entries[0].AuthorFolder, "Brandon Sanderson")
	}
	if entries[0].TitleFolder != "The Final Empire" {
		t.Errorf("TitleFolder=%q; want %q", entries[0].TitleFolder, "The Final Empire")
	}
}

func TestHandlerList_ScanError(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "no-such-dir"))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/api/library", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-existent library dir, got %d", rr.Code)
	}
}

func TestHandlerCleanup_All(t *testing.T) {
	libDir := t.TempDir()
	// Double space: valid on all OSes, collapsed to single by sanitizeName → NeedsRename = true.
	if err := os.MkdirAll(
		filepath.Join(libDir, "Brandon Sanderson", "Mistborn  The Final Empire"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(libDir)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", nil)
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleaned != 1 {
		t.Errorf("Cleaned=%d; want 1", resp.Cleaned)
	}
}

func TestHandlerCleanup_AllNothingToClean(t *testing.T) {
	libDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(libDir, "Frank Herbert", "Dune"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(libDir)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", nil)
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleaned != 0 {
		t.Errorf("Cleaned=%d; want 0", resp.Cleaned)
	}
}

func TestHandlerCleanup_SingleFound(t *testing.T) {
	libDir := t.TempDir()
	// Double space in folder name — valid on all OSes, triggers NeedsRename.
	if err := os.MkdirAll(
		filepath.Join(libDir, "Brandon Sanderson", "Mistborn  The Final Empire"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := NewHandler(libDir)
	// Request body must use the exact TitleFolder value that cleanupSingle matches against.
	body := `{"author":"Brandon Sanderson","title":"Mistborn  The Final Empire"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Cleaned != 1 {
		t.Errorf("Cleaned=%d; want 1", resp.Cleaned)
	}
}

func TestHandlerCleanup_SingleNotFound(t *testing.T) {
	h := NewHandler(t.TempDir())
	body := `{"author":"Nobody","title":"Nonexistent"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown book, got %d", rr.Code)
	}
}

func TestHandlerCleanup_MalformedJSON(t *testing.T) {
	h := NewHandler(t.TempDir())
	body := `{not valid json`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", rr.Code)
	}
}

func TestHandlerCleanup_SingleScanError(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "no-such-dir"))
	body := `{"author":"Author","title":"Title"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for scan error, got %d", rr.Code)
	}
}

// ── ABS merge integration in Cleanup ─────────────────────────────────────────

// mockABSClient is a controllable absLibraryClient for handler tests.
type mockABSClient struct {
	findItemFn  func(ctx context.Context, apiKey, title, author string) (string, error)
	mergePartFn func(ctx context.Context, apiKey, itemID string) error
	findCalls   []struct{ title, author string }
	mergeCalls  []string // item IDs passed to MergeMultiPart
}

func (m *mockABSClient) FindLibraryItemByTitleAuthor(ctx context.Context, apiKey, title, author string) (string, error) {
	m.findCalls = append(m.findCalls, struct{ title, author string }{title, author})
	if m.findItemFn != nil {
		return m.findItemFn(ctx, apiKey, title, author)
	}
	return "", nil
}

func (m *mockABSClient) MergeMultiPart(ctx context.Context, apiKey, itemID string) error {
	m.mergeCalls = append(m.mergeCalls, itemID)
	if m.mergePartFn != nil {
		return m.mergePartFn(ctx, apiKey, itemID)
	}
	return nil
}

func TestHandlerCleanup_SingleTriggersABSMergeForMultiPartBook(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"part1.mp3", "part2.mp3"} {
		if err := os.WriteFile(filepath.Join(titlePath, name), []byte("audio"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	merged := make(chan string, 1)
	mock := &mockABSClient{
		findItemFn: func(_ context.Context, _, _, _ string) (string, error) { return "li_abc", nil },
		mergePartFn: func(_ context.Context, _, itemID string) error {
			merged <- itemID
			return nil
		},
	}
	h := NewHandler(libDir)
	h.SetABSClient(mock, "test-key")

	body := `{"author":"Author","title":"Book"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Cleaned != 1 {
		t.Errorf("Cleaned=%d; want 1", resp.Cleaned)
	}
	// ABS merge is async; wait for it with a timeout.
	select {
	case itemID := <-merged:
		if itemID != "li_abc" {
			t.Errorf("MergeMultiPart called with %q; want li_abc", itemID)
		}
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for async ABS merge")
	}
}

func TestHandlerCleanup_SingleNoABSMergeForSingleAudioFile(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "book.m4b"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	mock := &mockABSClient{}
	h := NewHandler(libDir)
	h.SetABSClient(mock, "test-key")

	body := `{"author":"Author","title":"Book"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if len(mock.mergeCalls) != 0 {
		t.Errorf("expected no merge calls for single-file book; got %v", mock.mergeCalls)
	}
}

func TestHandlerCleanup_SingleABSMergeErrorIsLogged(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"p1.mp3", "p2.mp3"} {
		if err := os.WriteFile(filepath.Join(titlePath, name), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	done := make(chan struct{}, 1)
	mock := &mockABSClient{
		findItemFn: func(_ context.Context, _, _, _ string) (string, error) {
			done <- struct{}{}
			return "", errors.New("ABS unreachable")
		},
	}
	h := NewHandler(libDir)
	h.SetABSClient(mock, "test-key")

	body := `{"author":"Author","title":"Book"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	// Cleanup succeeds immediately; ABS error is async and only logged.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Cleaned != 1 {
		t.Errorf("Cleaned=%d; want 1", resp.Cleaned)
	}
	if len(resp.Errors) != 0 {
		t.Errorf("unexpected response errors: %v", resp.Errors)
	}
	// Wait for the async goroutine to call FindLibraryItemByTitleAuthor.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("timed out waiting for async ABS merge attempt")
	}
}

func TestHandlerCleanup_AllTriggersABSMergeOnlyForMultiPartBooks(t *testing.T) {
	libDir := t.TempDir()

	// Multi-part book with a dirty name (double space) so NeedsRename=true,
	// which is required for the all-path merge filter to fire.
	multiPath := filepath.Join(libDir, "Author A", "Multi  Book")
	if err := os.MkdirAll(multiPath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"p1.mp3", "p2.mp3"} {
		if err := os.WriteFile(filepath.Join(multiPath, n), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Single-part book (clean name, should never trigger merge).
	singlePath := filepath.Join(libDir, "Author B", "Single Book")
	if err := os.MkdirAll(singlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(singlePath, "book.m4b"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	// done is closed by the mock when MergeMultiPart is called, allowing the
	// test to synchronise with the background goroutine.
	done := make(chan struct{})
	mock := &mockABSClient{
		findItemFn:  func(_ context.Context, _, _, _ string) (string, error) { return "li_multi", nil },
		mergePartFn: func(_ context.Context, _, _ string) error { close(done); return nil },
	}
	h := NewHandler(libDir)
	h.SetABSClient(mock, "test-key")

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", nil)
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Cleaned != 1 {
		t.Errorf("Cleaned=%d; want 1", resp.Cleaned)
	}

	// Wait for the background merge goroutine.
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for background ABS merge")
	}
	if len(mock.mergeCalls) != 1 {
		t.Errorf("merge calls: %v; want exactly 1", mock.mergeCalls)
	}
}

func TestHandlerCleanup_SingleNoABSMergeWhenClientNotConfigured(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"p1.mp3", "p2.mp3"} {
		if err := os.WriteFile(filepath.Join(titlePath, n), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// No SetABSClient — ABS not configured; single-book cleanup should still succeed.
	h := NewHandler(libDir)
	body := `{"author":"Author","title":"Book"}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Cleaned != 1 {
		t.Errorf("Cleaned=%d; want 1", resp.Cleaned)
	}
}

func TestHandlerCleanup_AllRescanErrorReportedInErrors(t *testing.T) {
	// Point the handler at a directory that will disappear before the rescan.
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"p1.mp3", "p2.mp3"} {
		if err := os.WriteFile(filepath.Join(titlePath, n), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mock := &mockABSClient{
		findItemFn: func(_ context.Context, _, _, _ string) (string, error) { return "li_x", nil },
	}
	h := NewHandler(libDir)
	h.SetABSClient(mock, "test-key")

	// Remove the library directory so the ScanLibrary call inside CleanupAll fails.
	if err := os.RemoveAll(libDir); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", nil)
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Errors) == 0 {
		t.Error("expected rescan error in response Errors slice")
	}
}

func TestHandlerCleanup_NoABSMergeWhenClientNotConfigured(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"p1.mp3", "p2.mp3"} {
		if err := os.WriteFile(filepath.Join(titlePath, n), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// No SetABSClient — ABS not configured.
	h := NewHandler(libDir)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/cleanup", nil)
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp cleanupResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Cleaned != 0 {
		t.Errorf("Cleaned=%d; want 0 (multi-part book had clean name)", resp.Cleaned)
	}
}

func TestHandlerPrune_RemovesEmpty(t *testing.T) {
	libDir := t.TempDir()
	// Create an empty author/book hierarchy alongside a non-empty book.
	if err := os.MkdirAll(filepath.Join(libDir, "Ghost Author", "Empty Book"), 0o755); err != nil {
		t.Fatal(err)
	}
	keepDir := filepath.Join(libDir, "Real Author", "Real Book")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keepDir, "chapter.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(libDir)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Removed int `json:"removed"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// Ghost Author + Empty Book = 2 removed.
	if resp.Removed != 2 {
		t.Errorf("Removed=%d; want 2", resp.Removed)
	}
	// Empty dirs gone.
	if _, err := os.Stat(filepath.Join(libDir, "Ghost Author")); err == nil {
		t.Error("Ghost Author should have been removed")
	}
	// Non-empty dir preserved.
	if _, err := os.Stat(keepDir); err != nil {
		t.Errorf("Real Book dir should still exist: %v", err)
	}
}

func TestHandlerPrune_NothingToRemove(t *testing.T) {
	libDir := t.TempDir()
	keepDir := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(keepDir, "ch.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler(libDir)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Removed int `json:"removed"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Removed != 0 {
		t.Errorf("Removed=%d; want 0", resp.Removed)
	}
}

func TestHandlerPrune_NonExistentLibrary(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "no-such-dir"))
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/library/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for non-existent library dir, got %d", rr.Code)
	}
}
