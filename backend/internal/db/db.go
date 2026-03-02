// Package db wraps the SQLite connection and exposes typed query methods for
// users and requests. All public methods accept a context.Context so callers
// can propagate deadlines and cancellations.
package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // register the "sqlite" driver
)

// DB wraps *sql.DB with application-level query methods.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at dsn, sets connection pragmas,
// and runs any pending schema migrations. Returns a ready-to-use DB.
func Open(dsn string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite does not support concurrent writers; a single connection avoids
	// "database is locked" errors while WAL mode handles concurrent readers.
	sqlDB.SetMaxOpenConns(1)

	if _, err := sqlDB.ExecContext(context.Background(), pragmas); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("set sqlite pragmas: %w", err)
	}

	db := &DB{sqlDB}
	if err := db.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return db, nil
}

// pragmas are applied once on connection open.
const pragmas = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
PRAGMA busy_timeout = 5000;
`
