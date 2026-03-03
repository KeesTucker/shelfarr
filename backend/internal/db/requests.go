package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RequestStatus enumerates the lifecycle states of a download request.
type RequestStatus string

const (
	StatusPending     RequestStatus = "pending"
	StatusDownloading RequestStatus = "downloading"
	StatusMoving      RequestStatus = "moving"
	StatusDone        RequestStatus = "done"
	StatusFailed      RequestStatus = "failed"
)

// Request mirrors a row in the requests table.
type Request struct {
	ID           string
	UserID       string
	Title        string
	Author       string
	SearchQuery  string
	TorrentName  sql.NullString
	TorrentHash  sql.NullString
	Status       RequestStatus
	Error        sql.NullString
	MetadataJSON sql.NullString
	FinalPath    sql.NullString
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// RequestWithUser extends Request with the requesting user's username, used
// for the admin list view.
type RequestWithUser struct {
	Request
	Username string
}

// ── writes ────────────────────────────────────────────────────────────────────

// CreateRequest inserts a new request. At creation time only the identifying
// fields (id, user, title, author, search query) are required; torrent
// fields are populated once a torrent is selected.
func (db *DB) CreateRequest(ctx context.Context, r *Request) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO requests
			(id, user_id, title, author, search_query, torrent_name, torrent_hash, status)
		VALUES
			(?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.UserID, r.Title, r.Author, r.SearchQuery,
		r.TorrentName, r.TorrentHash, r.Status,
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	return nil
}

// UpdateRequestStatus updates the status column and optionally additional
// fields via functional options. Fields not covered by an option are left
// unchanged using COALESCE.
func (db *DB) UpdateRequestStatus(ctx context.Context, id string, status RequestStatus, opts ...UpdateOption) error {
	u := &updateFields{status: status}
	for _, opt := range opts {
		opt(u)
	}
	_, err := db.ExecContext(ctx, `
		UPDATE requests
		SET
			status        = ?,
			error         = COALESCE(?, error),
			metadata_json = COALESCE(?, metadata_json),
			final_path    = COALESCE(?, final_path),
			torrent_hash  = COALESCE(?, torrent_hash),
			torrent_name  = COALESCE(?, torrent_name),
			updated_at    = CURRENT_TIMESTAMP
		WHERE id = ?`,
		u.status,
		u.errMsg,
		u.metadataJSON,
		u.finalPath,
		u.torrentHash,
		u.torrentName,
		id,
	)
	if err != nil {
		return fmt.Errorf("update request status: %w", err)
	}
	return nil
}

// ── functional options for UpdateRequestStatus ────────────────────────────────

type updateFields struct {
	status       RequestStatus
	errMsg       sql.NullString
	metadataJSON sql.NullString
	finalPath    sql.NullString
	torrentHash  sql.NullString
	torrentName  sql.NullString
}

// UpdateOption is a functional option for UpdateRequestStatus.
type UpdateOption func(*updateFields)

// WithError attaches an error message to the update.
func WithError(msg string) UpdateOption {
	return func(u *updateFields) {
		u.errMsg = sql.NullString{String: msg, Valid: true}
	}
}

// WithMetadata attaches serialised metadata JSON to the update.
func WithMetadata(json string) UpdateOption {
	return func(u *updateFields) {
		u.metadataJSON = sql.NullString{String: json, Valid: true}
	}
}

// WithFinalPath attaches the resolved library path to the update.
func WithFinalPath(path string) UpdateOption {
	return func(u *updateFields) {
		u.finalPath = sql.NullString{String: path, Valid: true}
	}
}

// WithTorrentHash attaches the qBittorrent infohash to the update.
func WithTorrentHash(hash string) UpdateOption {
	return func(u *updateFields) {
		u.torrentHash = sql.NullString{String: hash, Valid: true}
	}
}

// WithTorrentName attaches the torrent display name to the update.
func WithTorrentName(name string) UpdateOption {
	return func(u *updateFields) {
		u.torrentName = sql.NullString{String: name, Valid: true}
	}
}

// ── reads ─────────────────────────────────────────────────────────────────────

// GetRequest retrieves a single request by its UUID. Returns ErrNotFound if
// no row matches.
func (db *DB) GetRequest(ctx context.Context, id string) (*Request, error) {
	row := db.QueryRowContext(ctx, selectRequestCols+` WHERE r.id = ?`, id)
	return scanRequest(row)
}

// GetRequestByHash retrieves a request by torrent infohash. Used by the
// background download watcher to correlate qBit progress updates with DB rows.
// Returns ErrNotFound if no row matches.
func (db *DB) GetRequestByHash(ctx context.Context, hash string) (*Request, error) {
	row := db.QueryRowContext(ctx, selectRequestCols+` WHERE r.torrent_hash = ?`, hash)
	return scanRequest(row)
}

// ListRequestsByUser returns all requests for a given user, newest first.
func (db *DB) ListRequestsByUser(ctx context.Context, userID string) ([]*Request, error) {
	rows, err := db.QueryContext(ctx,
		selectRequestCols+` WHERE r.user_id = ? ORDER BY r.created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list requests by user: %w", err)
	}
	return scanRequests(rows)
}

// ListAllRequestsWithUser returns all requests across all users, each annotated
// with the requesting user's username. Intended for the admin view.
func (db *DB) ListAllRequestsWithUser(ctx context.Context) ([]*RequestWithUser, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT
			r.id, r.user_id, r.title, r.author, r.search_query,
			r.torrent_name, r.torrent_hash, r.status, r.error,
			r.metadata_json, r.final_path, r.created_at, r.updated_at,
			u.username
		FROM requests r
		JOIN users u ON r.user_id = u.id
		ORDER BY r.created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list all requests with user: %w", err)
	}
	defer rows.Close()

	var result []*RequestWithUser
	for rows.Next() {
		var rw RequestWithUser
		r := &rw.Request
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Title, &r.Author, &r.SearchQuery,
			&r.TorrentName, &r.TorrentHash, &r.Status, &r.Error,
			&r.MetadataJSON, &r.FinalPath, &r.CreatedAt, &r.UpdatedAt,
			&rw.Username,
		); err != nil {
			return nil, fmt.Errorf("scan request row: %w", err)
		}
		result = append(result, &rw)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate requests: %w", err)
	}
	return result, nil
}

// ListTorrentNames returns all non-null torrent_name values across all
// requests. Used by the watch-dir scanner to exclude items already tracked.
func (db *DB) ListTorrentNames(ctx context.Context) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT torrent_name FROM requests WHERE torrent_name IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("list torrent names: %w", err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan torrent name: %w", err)
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// ListActiveDownloads returns all requests with status "downloading". Called
// on server startup so the watcher goroutine can resume monitoring in-flight
// downloads that survived a restart.
func (db *DB) ListActiveDownloads(ctx context.Context) ([]*Request, error) {
	rows, err := db.QueryContext(ctx,
		selectRequestCols+` WHERE r.status = 'downloading'`,
	)
	if err != nil {
		return nil, fmt.Errorf("list active downloads: %w", err)
	}
	return scanRequests(rows)
}

// ── scan helpers ──────────────────────────────────────────────────────────────

// selectRequestCols is the base SELECT for single-table request queries.
// The alias "r" must be preserved so WHERE clauses can reference columns.
const selectRequestCols = `
	SELECT
		r.id, r.user_id, r.title, r.author, r.search_query,
		r.torrent_name, r.torrent_hash, r.status, r.error,
		r.metadata_json, r.final_path, r.created_at, r.updated_at
	FROM requests r`

func scanRequest(row *sql.Row) (*Request, error) {
	var r Request
	err := row.Scan(
		&r.ID, &r.UserID, &r.Title, &r.Author, &r.SearchQuery,
		&r.TorrentName, &r.TorrentHash, &r.Status, &r.Error,
		&r.MetadataJSON, &r.FinalPath, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan request: %w", err)
	}
	return &r, nil
}

func scanRequests(rows *sql.Rows) ([]*Request, error) {
	defer rows.Close()
	var result []*Request
	for rows.Next() {
		var r Request
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.Title, &r.Author, &r.SearchQuery,
			&r.TorrentName, &r.TorrentHash, &r.Status, &r.Error,
			&r.MetadataJSON, &r.FinalPath, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan request row: %w", err)
		}
		result = append(result, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate requests: %w", err)
	}
	return result, nil
}
