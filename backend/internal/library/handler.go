package library

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"shelfarr/internal/respond"
)

// absLibraryClient is the subset of the ABS client used by the library handler.
type absLibraryClient interface {
	FindLibraryItemByTitleAuthor(ctx context.Context, apiKey, title, author string) (string, error)
	MergeMultiPart(ctx context.Context, apiKey, itemID string) error
}

// Handler serves the library management API endpoints.
type Handler struct {
	libraryDir string
	absClient  absLibraryClient
	absAPIKey  string
}

// NewHandler creates a Handler for the given library directory.
func NewHandler(libraryDir string) *Handler {
	return &Handler{libraryDir: libraryDir}
}

// SetABSClient configures the ABS client and API key used to trigger
// automatic multi-part merge after cleanup. Call this when ABS_API_KEY is set.
func (h *Handler) SetABSClient(client absLibraryClient, apiKey string) {
	h.absClient = client
	h.absAPIKey = apiKey
}

// List handles GET /api/library. Returns all book entries found in libraryDir.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	entries, err := ScanLibrary(h.libraryDir)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "scan library: "+err.Error())
		return
	}
	if entries == nil {
		entries = []BookEntry{}
	}
	respond.JSON(w, http.StatusOK, entries)
}

// cleanupRequest is the optional body for POST /api/library/cleanup.
// If Author and Title are both set, only that book is cleaned.
// Otherwise all mismatched books are cleaned.
type cleanupRequest struct {
	Author string `json:"author"`
	Title  string `json:"title"`
}

type cleanupResponse struct {
	Cleaned int      `json:"cleaned"`
	Errors  []string `json:"errors"`
}

// Cleanup handles POST /api/library/cleanup.
func (h *Handler) Cleanup(w http.ResponseWriter, r *http.Request) {
	var req cleanupRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respond.Error(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}

	if req.Author != "" && req.Title != "" {
		h.cleanupSingle(w, r, req.Author, req.Title)
		return
	}

	entries, cleaned, errs := CleanupAll(h.libraryDir)
	if h.absClient != nil && h.absAPIKey != "" {
		ctx := context.WithoutCancel(r.Context())
		go h.mergeMultiPartEntries(ctx, entries)
	}
	respond.JSON(w, http.StatusOK, cleanupResponse{Cleaned: cleaned, Errors: errs})
}

// Prune handles POST /api/library/prune. Removes empty directories from the
// library tree and returns how many were deleted.
func (h *Handler) Prune(w http.ResponseWriter, r *http.Request) {
	removed, err := pruneEmptyDirs(h.libraryDir)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "prune: "+err.Error())
		return
	}
	respond.JSON(w, http.StatusOK, struct {
		Removed int `json:"removed"`
	}{Removed: removed})
}

// cleanupSingle cleans a single book and triggers ABS merge if it is multi-part.
func (h *Handler) cleanupSingle(w http.ResponseWriter, r *http.Request, author, title string) {
	entries, err := ScanLibrary(h.libraryDir)
	if err != nil {
		respond.Error(w, http.StatusInternalServerError, "scan library: "+err.Error())
		return
	}

	for _, e := range entries {
		if e.AuthorFolder == author && e.TitleFolder == title {
			needsFileWork := e.NeedsRename || e.NeedsFlat
			cleaned := 0
			if needsFileWork {
				if err := CleanupBook(h.libraryDir, e); err != nil {
					respond.Error(w, http.StatusInternalServerError, err.Error())
					return
				}
				cleaned = 1
			}
			if e.NeedsEncode && h.absClient != nil && h.absAPIKey != "" {
				ctx := context.WithoutCancel(r.Context())
				go h.mergeMultiPartEntries(ctx, []BookEntry{e})
			}
			respond.JSON(w, http.StatusOK, cleanupResponse{Cleaned: cleaned, Errors: nil})
			return
		}
	}
	respond.Error(w, http.StatusNotFound, "book not found")
}

// mergeMultiPartEntries triggers ABS encode-m4b for every book in entries that
// needs encoding. Runs in a background goroutine; errors are logged rather than returned.
func (h *Handler) mergeMultiPartEntries(ctx context.Context, entries []BookEntry) {
	for _, e := range entries {
		if !e.NeedsEncode {
			continue
		}
		title, author := e.absLookupKeys()
		if err := h.triggerABSMerge(ctx, title, author); err != nil {
			slog.Warn("library: ABS merge", "author", e.AuthorFolder, "title", e.TitleFolder, "err", err)
		}
	}
}

// absLookupKeys returns the title and author to use when searching ABS.
// When metadata comes from folder names, ExpectedTitle/Author are the
// sanitised canonical forms; Metadata.Title/Author are the raw (dirty) folder
// names that may not match what ABS has stored.
func (e BookEntry) absLookupKeys() (title, author string) {
	if e.MetadataSource == "folder" {
		return e.ExpectedTitle, e.ExpectedAuthor
	}
	return e.Metadata.Title, e.Metadata.Author
}

// triggerABSMerge finds the ABS library item by title+author and triggers merge.
// Callers must ensure h.absClient and h.absAPIKey are set before calling.
func (h *Handler) triggerABSMerge(ctx context.Context, title, author string) error {
	itemID, err := h.absClient.FindLibraryItemByTitleAuthor(ctx, h.absAPIKey, title, author)
	if err != nil {
		return fmt.Errorf("find ABS item: %w", err)
	}
	if itemID == "" {
		return fmt.Errorf("item %q by %q not found in ABS library", title, author)
	}
	return h.absClient.MergeMultiPart(ctx, h.absAPIKey, itemID)
}
