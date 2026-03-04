package auth

import (
	"context"
	"net/http"
)

type contextKey string

const claimsKey contextKey = "auth_claims"

// Authenticate is a chi middleware that extracts and validates the JWT from
// the httpOnly auth cookie. On success the parsed Claims are stored in the
// request context for downstream handlers to read via ClaimsFromContext.
// Returns 401 if the cookie is absent or the token is invalid/expired.
func Authenticate(cfg TokenConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(authCookieName)
			if err != nil {
				http.Error(w, "missing auth cookie", http.StatusUnauthorized)
				return
			}

			claims, err := ParseToken(cfg, cookie.Value)
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
