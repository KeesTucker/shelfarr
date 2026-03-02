// Package requests provides HTTP handlers for the /api/requests endpoints.
package requests

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"bookarr/internal/auth"
	"bookarr/internal/db"
	"bookarr/internal/prowlarr"
	"bookarr/internal/qbit"
	"bookarr/internal/respond"
)

// Handler handles the /api/requests routes.
type Handler struct {
	db       *db.DB
	prowlarr *prowlarr.Client
	qbit     *qbit.Client
	savePath string // DOWNLOAD_DIR
	category string // QBIT_CATEGORY (empty = uncategorised)
}

// New creates a Handler wired to the given dependencies.
func New(database *db.DB, p *prowlarr.Client, q *qbit.Client, savePath, category string) *Handler {
	return &Handler{
		db:       database,
		prowlarr: p,
		qbit:     q,
		savePath: savePath,
		category: category,
	}
}

// submitBody is the request body accepted by Submit.
type submitBody struct {
	Title       string `json:"title"`
	Author      string `json:"author"`
	TorrentGUID string `json:"torrentGuid"`
}

// requestResponse is the JSON shape for a single request in API responses.
type requestResponse struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	Status      string    `json:"status"`
	TorrentName *string   `json:"torrentName,omitempty"`
	TorrentHash *string   `json:"torrentHash,omitempty"`
	Error       *string   `json:"error,omitempty"`
	FinalPath   *string   `json:"finalPath,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// requestWithUserResponse adds the requesting user's username to requestResponse.
// Returned only on the admin list view.
type requestWithUserResponse struct {
	requestResponse
	Username string `json:"username"`
}

// Submit handles POST /api/requests.
//
// Flow:
//  1. Validate request body.
//  2. Look up the selected torrent release from the Prowlarr search cache.
//  3. Send the torrent/magnet to qBittorrent.
//  4. Persist a DB record with status "downloading".
//  5. Return the created request.
func (h *Handler) Submit(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())

	var body submitBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.Title == "" || body.Author == "" || body.TorrentGUID == "" {
		respond.Error(w, http.StatusBadRequest, "title, author, and torrentGuid are required")
		return
	}

	// Fetch release from the in-memory Prowlarr search cache.
	release, ok := h.prowlarr.GetByGUID(body.TorrentGUID)
	if !ok {
		respond.Error(w, http.StatusBadRequest, "torrent GUID not found — please search again")
		return
	}

	// Add torrent to qBittorrent and retrieve the assigned infohash.
	hash, err := h.qbit.AddTorrent(r.Context(), release.DownloadURL, h.savePath, h.category)
	if err != nil {
		slog.Error("add torrent to qbit", "guid", body.TorrentGUID, "err", err)
		respond.Error(w, http.StatusBadGateway, "could not add torrent to qBittorrent")
		return
	}

	// Persist the request record.
	req := &db.Request{
		ID:          uuid.NewString(),
		UserID:      claims.UserID,
		Title:       body.Title,
		Author:      body.Author,
		SearchQuery: body.Title + " " + body.Author,
		TorrentName: sql.NullString{String: release.Title, Valid: true},
		TorrentHash: sql.NullString{String: hash, Valid: true},
		Status:      db.StatusDownloading,
	}
	if err := h.db.CreateRequest(r.Context(), req); err != nil {
		slog.Error("create request", "user_id", claims.UserID, "err", err)
		respond.Error(w, http.StatusInternalServerError, "failed to save request")
		return
	}

	respond.JSON(w, http.StatusCreated, toResponse(req))
}

// List handles GET /api/requests.
// Admin users receive all requests across all users (with username).
// Regular users receive only their own requests.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())

	if claims.Role == "admin" {
		rows, err := h.db.ListAllRequestsWithUser(r.Context())
		if err != nil {
			slog.Error("list all requests", "err", err)
			respond.Error(w, http.StatusInternalServerError, "failed to list requests")
			return
		}
		out := make([]requestWithUserResponse, 0, len(rows))
		for _, rw := range rows {
			out = append(out, requestWithUserResponse{
				requestResponse: toResponse(&rw.Request),
				Username:        rw.Username,
			})
		}
		respond.JSON(w, http.StatusOK, out)
		return
	}

	rows, err := h.db.ListRequestsByUser(r.Context(), claims.UserID)
	if err != nil {
		slog.Error("list requests by user", "user_id", claims.UserID, "err", err)
		respond.Error(w, http.StatusInternalServerError, "failed to list requests")
		return
	}
	out := make([]requestResponse, 0, len(rows))
	for _, req := range rows {
		out = append(out, toResponse(req))
	}
	respond.JSON(w, http.StatusOK, out)
}

// Get handles GET /api/requests/:id.
// Non-admin users may only retrieve their own requests.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	id := chi.URLParam(r, "id")

	req, err := h.db.GetRequest(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "request not found")
			return
		}
		slog.Error("get request", "id", id, "err", err)
		respond.Error(w, http.StatusInternalServerError, "failed to get request")
		return
	}

	if claims.Role != "admin" && req.UserID != claims.UserID {
		respond.Error(w, http.StatusForbidden, "forbidden")
		return
	}

	respond.JSON(w, http.StatusOK, toResponse(req))
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toResponse(r *db.Request) requestResponse {
	res := requestResponse{
		ID:        r.ID,
		Title:     r.Title,
		Author:    r.Author,
		Status:    string(r.Status),
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if r.TorrentName.Valid {
		res.TorrentName = &r.TorrentName.String
	}
	if r.TorrentHash.Valid {
		res.TorrentHash = &r.TorrentHash.String
	}
	if r.Error.Valid {
		res.Error = &r.Error.String
	}
	if r.FinalPath.Valid {
		res.FinalPath = &r.FinalPath.String
	}
	return res
}
