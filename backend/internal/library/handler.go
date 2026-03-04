package library

import (
	"encoding/json"
	"net/http"

	"shelfarr/internal/respond"
)

// Handler serves the library management API endpoints.
type Handler struct {
	libraryDir string
}

// NewHandler creates a Handler for the given library directory.
func NewHandler(libraryDir string) *Handler {
	return &Handler{libraryDir: libraryDir}
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
		h.cleanupSingle(w, req.Author, req.Title)
		return
	}

	cleaned, errs := CleanupAll(h.libraryDir)
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

func (h *Handler) cleanupSingle(w http.ResponseWriter, author, title string) {
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
			respond.JSON(w, http.StatusOK, cleanupResponse{Cleaned: 1})
			return
		}
	}
	respond.Error(w, http.StatusNotFound, "book not found")
}
