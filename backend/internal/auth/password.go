// Package auth provides JWT token handling, bcrypt password helpers, and
// HTTP middleware for protecting authenticated routes.
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost is the work factor for password hashing. DefaultCost (10) is
// intentionally slow enough to resist brute-force but fast enough for normal
// login traffic.
const bcryptCost = bcrypt.DefaultCost

// HashPassword returns a bcrypt hash of password suitable for storage.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword reports whether password matches the stored bcrypt hash.
// Returns false on any mismatch or error; callers need not distinguish the two.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
