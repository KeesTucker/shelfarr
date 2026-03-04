// Package requests provides HTTP handlers for the /api/requests endpoints.
package requests

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"shelfarr/internal/auth"
	"shelfarr/internal/db"
	"shelfarr/internal/metadata"
	"shelfarr/internal/prowlarr"
	"shelfarr/internal/qbit"
	"shelfarr/internal/respond"
)

// Handler handles the /api/requests routes.
type Handler struct {
	db       *db.DB
	prowlarr *prowlarr.Client
	qbit     *qbit.Client
	category string // QBIT_CATEGORY (empty = uncategorised)

	// deleteFromQBit controls whether deleting a "downloading" request also
	// removes its torrent from qBittorrent (without deleting files). When
	// false, deleting an active download returns 409 Conflict.
	deleteFromQBit bool

	// optional import fields — set via SetImportConfig.
	ctx      context.Context // server lifetime context; cancelled on shutdown
	watchDir string
	onImport func(ctx context.Context, req *db.Request, torrentName string) error
	onFail   func(ctx context.Context, req *db.Request, reason string)
	launch   func(func())
}

// New creates a Handler wired to the given dependencies.
func New(database *db.DB, p *prowlarr.Client, q *qbit.Client, category string) *Handler {
	return &Handler{
		db:       database,
		prowlarr: p,
		qbit:     q,
		category: category,
		launch:   func(f func()) { go f() },
	}
}

// SetDeleteTorrentsOnRequestDelete controls whether deleting a request also
// removes the associated torrent from qBittorrent (without deleting files).
// Only applies to completed/failed requests — active downloads (status
// "downloading") are always blocked from deletion regardless of this setting.
// Default is false.
func (h *Handler) SetDeleteTorrentsOnRequestDelete(enabled bool) {
	h.deleteFromQBit = enabled
}

// SetImportConfig wires the optional file-import functionality.
// ctx is the server lifetime context (cancelled on shutdown); watchDir is the
// directory to scan for untracked files; onImport runs the move pipeline for
// each import; onFail is called (best-effort) on error.
func (h *Handler) SetImportConfig(
	ctx context.Context,
	watchDir string,
	onImport func(ctx context.Context, req *db.Request, torrentName string) error,
	onFail func(ctx context.Context, req *db.Request, reason string),
) {
	h.ctx = ctx
	h.watchDir = watchDir
	h.onImport = onImport
	h.onFail = onFail
}

// submitBody is the request body accepted by Submit.
type submitBody struct {
	Title       string         `json:"title"`
	Author      string         `json:"author"`
	TorrentGUID string         `json:"torrentGuid"`
	Metadata    *metadata.Book `json:"metadata,omitempty"`
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

	// When AutoTMM is enabled qBittorrent determines the save path from the
	// category configuration, so we skip fetching it. Without AutoTMM we need
	// the default save path so qBit knows where to put the files.
	var (
		savePath string
		err      error
	)
	if !h.qbit.AutoTMM() {
		savePath, err = h.qbit.GetDefaultSavePath(r.Context())
		if err != nil {
			slog.Error("get qbit save path", "err", err)
			respond.Error(w, http.StatusBadGateway, "could not determine qBittorrent download path")
			return
		}
	}

	// Add torrent to qBittorrent and retrieve the assigned infohash.
	hash, err := h.qbit.AddTorrent(r.Context(), release.DownloadURL, savePath, h.category)
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
	if body.Metadata != nil {
		metaJSON, err := body.Metadata.JSON()
		if err != nil {
			slog.Error("marshal user-selected metadata", "err", err)
			respond.Error(w, http.StatusBadRequest, "invalid metadata")
			return
		}
		req.MetadataJSON = sql.NullString{String: metaJSON, Valid: true}
	}
	if err := h.db.CreateRequest(r.Context(), req); err != nil {
		slog.Error("create request", "user_id", claims.UserID, "err", err) //nolint:gosec
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
		slog.Error("list requests by user", "user_id", claims.UserID, "err", err) //nolint:gosec
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
		slog.Error("get request", "id", id, "err", err) //nolint:gosec
		respond.Error(w, http.StatusInternalServerError, "failed to get request")
		return
	}

	if claims.Role != "admin" && req.UserID != claims.UserID {
		respond.Error(w, http.StatusForbidden, "forbidden")
		return
	}

	respond.JSON(w, http.StatusOK, toResponse(req))
}

// Delete handles DELETE /api/requests/:id.
// Admins may delete any request. Regular users may only delete their own.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	claims, _ := auth.ClaimsFromContext(r.Context())
	id := chi.URLParam(r, "id")

	req, err := h.db.GetRequest(r.Context(), id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respond.Error(w, http.StatusNotFound, "request not found")
			return
		}
		slog.Error("delete request: get", "id", id, "err", err) //nolint:gosec
		respond.Error(w, http.StatusInternalServerError, "failed to get request")
		return
	}

	if claims.Role != "admin" && req.UserID != claims.UserID {
		respond.Error(w, http.StatusForbidden, "forbidden")
		return
	}

	switch req.Status {
	case db.StatusImporting:
		// A goroutine is actively importing files; blocking deletion avoids a
		// silent update-on-deleted-row race.
		respond.Error(w, http.StatusConflict, "request is currently being processed; wait for it to finish")
		return
	case db.StatusDownloading:
		// The watcher is tracking this torrent; deleting the record mid-download
		// would leave an orphaned torrent in qBittorrent with no DB entry.
		respond.Error(w, http.StatusConflict, "request is currently downloading; wait for it to finish")
		return
	}

	// Best-effort: remove the torrent from qBit when cleanup is enabled and
	// the request has a known hash (covers done, failed).
	if h.deleteFromQBit && req.TorrentHash.Valid {
		if err := h.qbit.RemoveTorrent(r.Context(), req.TorrentHash.String); err != nil {
			slog.Warn("delete request: remove torrent from qBit", "id", id, "hash", req.TorrentHash.String, "err", err) //nolint:gosec
		}
	}

	if err := h.db.DeleteRequest(r.Context(), id); err != nil {
		slog.Error("delete request", "id", id, "err", err) //nolint:gosec
		respond.Error(w, http.StatusInternalServerError, "failed to delete request")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── import ────────────────────────────────────────────────────────────────────

// watchDirEntry is a single item returned by ListWatchDir.
type watchDirEntry struct {
	Name string `json:"name"`
}

// importBody is the request body accepted by Import.
type importBody struct {
	TorrentName string `json:"torrentName"`
	Title       string `json:"title"`
	Author      string `json:"author"`
}

// ListWatchDir handles GET /api/watchdir. Admin only (enforced at router).
// Returns top-level entries in the watch directory that don't already have a
// corresponding torrent_name in the database.
func (h *Handler) ListWatchDir(w http.ResponseWriter, r *http.Request) {
	if h.watchDir == "" {
		respond.Error(w, http.StatusNotImplemented, "watch dir not configured")
		return
	}

	entries, err := os.ReadDir(h.watchDir)
	if err != nil {
		slog.Error("list watch dir", "path", h.watchDir, "err", err)
		respond.Error(w, http.StatusInternalServerError, "could not read watch directory")
		return
	}

	known, err := h.db.ListTorrentNames(r.Context())
	if err != nil {
		slog.Error("list torrent names for watchdir", "err", err)
		respond.Error(w, http.StatusInternalServerError, "could not query database")
		return
	}
	knownSet := make(map[string]bool, len(known))
	for _, n := range known {
		knownSet[n] = true
	}

	out := make([]watchDirEntry, 0, len(entries))
	for _, e := range entries {
		if !knownSet[e.Name()] {
			out = append(out, watchDirEntry{Name: e.Name()})
		}
	}
	respond.JSON(w, http.StatusOK, out)
}

// Import handles POST /api/import. Admin only (enforced at router).
// Creates a request record in "importing" status and asynchronously runs the
// import pipeline (metadata lookup → hardlink/copy to library → Discord notification).
func (h *Handler) Import(w http.ResponseWriter, r *http.Request) {
	if h.onImport == nil {
		respond.Error(w, http.StatusNotImplemented, "import not configured")
		return
	}

	claims, _ := auth.ClaimsFromContext(r.Context())

	var body importBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.TorrentName == "" || body.Title == "" || body.Author == "" {
		respond.Error(w, http.StatusBadRequest, "torrentName, title, and author are required")
		return
	}

	req := &db.Request{
		ID:          uuid.NewString(),
		UserID:      claims.UserID,
		Title:       body.Title,
		Author:      body.Author,
		SearchQuery: body.Title + " " + body.Author,
		TorrentName: sql.NullString{String: body.TorrentName, Valid: true},
		Status:      db.StatusImporting,
	}
	if err := h.db.CreateRequest(r.Context(), req); err != nil {
		slog.Error("import: create request", "user_id", claims.UserID, "err", err) //nolint:gosec
		respond.Error(w, http.StatusInternalServerError, "failed to save request")
		return
	}

	h.runImport(req)
	respond.JSON(w, http.StatusAccepted, toResponse(req))
}

// runImport launches the import pipeline in a background goroutine. It mirrors
// the watcher's handleComplete pattern: onImport → StatusDone, or StatusFailed
// + onFail on error. Uses the server lifetime context so the goroutine is
// cancelled on shutdown rather than being tied to the originating HTTP request.
func (h *Handler) runImport(req *db.Request) {
	ctx := h.ctx
	h.launch(func() {
		if err := h.onImport(ctx, req, req.TorrentName.String); err != nil {
			slog.Error("import: pipeline failed", "request_id", req.ID, "err", err)
			_ = h.db.UpdateRequestStatus(ctx, req.ID, db.StatusFailed, db.WithError(err.Error()))
			if h.onFail != nil {
				h.onFail(ctx, req, err.Error())
			}
			return
		}
		if err := h.db.UpdateRequestStatus(ctx, req.ID, db.StatusDone); err != nil {
			slog.Error("import: set status done", "request_id", req.ID, "err", err)
			return
		}
		slog.Info("import: done", "request_id", req.ID)
	})
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
