package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrNotFound is returned when a query matches no rows.
var ErrNotFound = errors.New("not found")

// User mirrors a row in the users table.
type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// CreateUser inserts a new user record.
func (db *DB) CreateUser(ctx context.Context, id, username, passwordHash, role string) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)`,
		id, username, passwordHash, role,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// GetUserByUsername retrieves a user by their username. Returns ErrNotFound if
// no matching row exists.
func (db *DB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at
		 FROM users WHERE username = ?`,
		username,
	)
	return scanUser(row)
}

// GetUserByID retrieves a user by their UUID primary key. Returns ErrNotFound
// if no matching row exists.
func (db *DB) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := db.QueryRowContext(ctx,
		`SELECT id, username, password_hash, role, created_at
		 FROM users WHERE id = ?`,
		id,
	)
	return scanUser(row)
}

// CountUsers returns the total number of users in the database.
func (db *DB) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func scanUser(row *sql.Row) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}
