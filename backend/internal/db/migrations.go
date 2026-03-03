package db

import (
	"context"
	"fmt"
	"log/slog"
)

type migration struct {
	version int
	sql     string
}

// migrations is the ordered list of schema versions. Append new entries here;
// never edit existing ones once deployed.
var migrations = []migration{
	{version: 1, sql: schemaV1},
}

// migrate ensures the schema_migrations table exists, then applies any
// migrations that have not yet been recorded, each in its own transaction.
func (db *DB) migrate(ctx context.Context) error {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	for _, m := range migrations {
		var applied int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, m.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", m.version, err)
		}
		if applied > 0 {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %d: %w", m.version, err)
		}

		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %d: %w", m.version, err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version) VALUES (?)`, m.version,
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}

		slog.Info("applied db migration", "version", m.version)
	}

	return nil
}

// schemaV1 is the initial database schema.
const schemaV1 = `
CREATE TABLE users (
	id            TEXT PRIMARY KEY,
	username      TEXT UNIQUE NOT NULL,
	password_hash TEXT NOT NULL,
	role          TEXT NOT NULL DEFAULT 'user',
	created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE requests (
	id            TEXT PRIMARY KEY,
	user_id       TEXT NOT NULL REFERENCES users(id),
	title         TEXT NOT NULL,
	author        TEXT NOT NULL,
	search_query  TEXT NOT NULL,
	torrent_name  TEXT,
	torrent_hash  TEXT,
	status        TEXT NOT NULL DEFAULT 'pending',
	error         TEXT,
	metadata_json TEXT,
	final_path    TEXT,
	created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
	updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_requests_user_id      ON requests(user_id);
CREATE INDEX idx_requests_status       ON requests(status);
CREATE INDEX idx_requests_torrent_hash ON requests(torrent_hash);
`
