package library

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandlerList_EmptyLibrary(t *testing.T) {
	h := NewHandler(t.TempDir())
	req := httptest.NewRequest(http.MethodGet, "/api/library", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/library", nil)
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
	req := httptest.NewRequest(http.MethodGet, "/api/library", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/api/library/cleanup", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/api/library/cleanup", nil)
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
	req := httptest.NewRequest(http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
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
	req := httptest.NewRequest(http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown book, got %d", rr.Code)
	}
}

func TestHandlerCleanup_SingleScanError(t *testing.T) {
	h := NewHandler(filepath.Join(t.TempDir(), "no-such-dir"))
	body := `{"author":"Author","title":"Title"}`
	req := httptest.NewRequest(http.MethodPost, "/api/library/cleanup", strings.NewReader(body))
	req.ContentLength = int64(len(body))
	rr := httptest.NewRecorder()
	h.Cleanup(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for scan error, got %d", rr.Code)
	}
}
