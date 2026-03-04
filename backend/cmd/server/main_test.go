package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"shelfarr/internal/abs"
	"shelfarr/internal/auth"
	"shelfarr/internal/db"
	"shelfarr/internal/library"
	"shelfarr/internal/metadata"
	"shelfarr/internal/prowlarr"
	"shelfarr/internal/qbit"
	"shelfarr/internal/requests"
)

func newTestRouter(t *testing.T) (http.Handler, auth.TokenConfig) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	cfg := auth.TokenConfig{Secret: []byte("test-secret"), Expiry: time.Hour}
	rh := requests.New(d, prowlarr.New("", ""), qbit.New("", "", ""), "")
	mh := metadata.NewHandler(metadata.New())
	lh := library.NewHandler(t.TempDir())
	return buildRouter(d, cfg, abs.New(""), prowlarr.New("", ""), rh, mh, lh, t.TempDir()), cfg
}

// TestProtectedRoutesRequireAuth verifies every JWT-protected route returns
// 401 when called without an Authorization header.
func TestProtectedRoutesRequireAuth(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/auth/me"},
		{http.MethodGet, "/api/search"},
		{http.MethodGet, "/api/metadata/search"},
		{http.MethodPost, "/api/requests"},
		{http.MethodGet, "/api/requests"},
		{http.MethodGet, "/api/requests/some-id"},
		{http.MethodDelete, "/api/requests/some-id"},
		{http.MethodGet, "/api/watchdir"},
		{http.MethodPost, "/api/import"},
		{http.MethodGet, "/api/library"},
		{http.MethodPost, "/api/library/cleanup"},
		{http.MethodPost, "/api/library/prune"},
	}

	router, _ := newTestRouter(t)

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", rr.Code)
			}
		})
	}
}

// TestAdminOnlyRoutesRequireAdmin verifies that routes restricted to admins
// return 403 when called with a valid non-admin token.
func TestAdminOnlyRoutesRequireAdmin(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/watchdir"},
		{http.MethodPost, "/api/import"},
		{http.MethodGet, "/api/library"},
		{http.MethodPost, "/api/library/cleanup"},
		{http.MethodPost, "/api/library/prune"},
	}

	router, cfg := newTestRouter(t)

	userToken, err := auth.NewToken(cfg, "user-1", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: userToken})
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code != http.StatusForbidden {
				t.Errorf("expected 403, got %d", rr.Code)
			}
		})
	}
}

// TestPublicRoutes confirms public endpoints do not require a token.
func TestPublicRoutes(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/auth/login"},
		{http.MethodPost, "/api/auth/logout"},
	}

	router, _ := newTestRouter(t)

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)
			if rr.Code == http.StatusUnauthorized {
				t.Errorf("%s %s should be public, got 401", tc.method, tc.path)
			}
		})
	}
}
