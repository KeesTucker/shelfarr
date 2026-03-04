package metadata

import (
	"log/slog"
	"net/http"

	"shelfarr/internal/respond"
)

// Handler exposes metadata search over HTTP.
type Handler struct {
	client *Client
}

// NewHandler returns a Handler backed by the given Client.
func NewHandler(c *Client) *Handler {
	return &Handler{client: c}
}

// Search handles GET /api/metadata/search?title=X&author=Y.
// Returns up to 5 Book candidates as JSON, or an empty array when none found.
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	title := r.URL.Query().Get("title")
	author := r.URL.Query().Get("author")
	if title == "" {
		respond.Error(w, http.StatusBadRequest, "title query param is required")
		return
	}

	books, err := h.client.Search(r.Context(), title, author)
	if err != nil {
		slog.Warn("metadata: search failed", "title", title, "author", author, "err", err)
		// Return empty list rather than an error — search is best-effort.
		respond.JSON(w, http.StatusOK, []Book{})
		return
	}
	if books == nil {
		books = []Book{}
	}
	respond.JSON(w, http.StatusOK, books)
}
