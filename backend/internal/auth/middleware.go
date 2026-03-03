package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const claimsKey contextKey = "auth_claims"

// Authenticate is a chi middleware that extracts and validates the Bearer JWT
// from the Authorization header. On success the parsed Claims are stored in
// the request context for downstream handlers to read via ClaimsFromContext.
// Returns 401 if the header is absent or the token is invalid/expired.
func Authenticate(cfg TokenConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr := extractBearer(r)
			if tokenStr == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			claims, err := ParseToken(cfg, tokenStr)
			if err != nil {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext retrieves the JWT claims stored by the Authenticate
// middleware. Returns (nil, false) if the middleware was not applied.
func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(*Claims)
	return claims, ok
}

// RequireAdmin is a chi middleware that allows only users with role "admin".
// Must be used after Authenticate.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := ClaimsFromContext(r.Context())
		if !ok || claims.Role != "admin" {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func extractBearer(r *http.Request) string {
	token, found := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !found {
		return ""
	}
	return token
}
