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
	Merged  int      `json:"merged"`
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

	cleaned, errs := CleanupAll(h.libraryDir)
	merged := h.mergeAllMultiPart(r.Context(), &errs)
	respond.JSON(w, http.StatusOK, cleanupResponse{Cleaned: cleaned, Merged: merged, Errors: errs})
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
			if err := CleanupBook(h.libraryDir, e); err != nil {
				respond.Error(w, http.StatusInternalServerError, err.Error())
				return
			}
			merged := 0
			var mergeErrs []string
			if e.IsMultiPart {
				if err := h.triggerABSMerge(r.Context(), e.Metadata.Title, e.Metadata.Author); err != nil {
					slog.Warn("library: ABS merge", "author", e.AuthorFolder, "title", e.TitleFolder, "err", err)
					mergeErrs = append(mergeErrs, "ABS merge: "+err.Error())
				} else {
					merged = 1
				}
			}
			respond.JSON(w, http.StatusOK, cleanupResponse{Cleaned: 1, Merged: merged, Errors: mergeErrs})
			return
		}
	}
	respond.Error(w, http.StatusNotFound, "book not found")
}

// mergeAllMultiPart scans the library and triggers ABS merge for every
// multi-part book. It appends any errors to errs and returns the merge count.
func (h *Handler) mergeAllMultiPart(ctx context.Context, errs *[]string) int {
	if h.absClient == nil || h.absAPIKey == "" {
		return 0
	}
	entries, err := ScanLibrary(h.libraryDir)
	if err != nil {
		return 0
	}
	merged := 0
	for _, e := range entries {
		if !e.IsMultiPart {
			continue
		}
		if err := h.triggerABSMerge(ctx, e.Metadata.Title, e.Metadata.Author); err != nil {
			slog.Warn("library: ABS merge", "author", e.AuthorFolder, "title", e.TitleFolder, "err", err)
			*errs = append(*errs, fmt.Sprintf("ABS merge %s/%s: %s", e.AuthorFolder, e.TitleFolder, err))
		} else {
			merged++
		}
	}
	return merged
}

// triggerABSMerge finds the ABS library item by title+author and triggers merge.
func (h *Handler) triggerABSMerge(ctx context.Context, title, author string) error {
	if h.absClient == nil || h.absAPIKey == "" {
		return nil
	}
	itemID, err := h.absClient.FindLibraryItemByTitleAuthor(ctx, h.absAPIKey, title, author)
	if err != nil {
		return fmt.Errorf("find ABS item: %w", err)
	}
	if itemID == "" {
		return fmt.Errorf("item %q by %q not found in ABS library", title, author)
	}
	return h.absClient.MergeMultiPart(ctx, h.absAPIKey, itemID)
}
