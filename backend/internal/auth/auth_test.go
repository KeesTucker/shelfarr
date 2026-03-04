package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"shelfarr/internal/abs"
	"shelfarr/internal/auth"
	"shelfarr/internal/db"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func testTokenCfg() auth.TokenConfig {
	return auth.TokenConfig{Secret: []byte("test-secret"), Expiry: time.Hour}
}

// seedUser inserts a user and returns their ID.
func seedUser(t *testing.T, d *db.DB, username, password, role string) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	id := "user-" + username
	if err := d.CreateUser(context.Background(), id, username, hash, role); err != nil {
		t.Fatal(err)
	}
	return id
}

// absStub is a test double for the ABS authenticator.
type absStub struct {
	user *abs.User
	err  error
}

func (s absStub) Login(_ context.Context, _, _ string) (*abs.User, error) {
	return s.user, s.err
}

// ── password ──────────────────────────────────────────────────────────────────

func TestHashPassword(t *testing.T) {
	hash, err := auth.HashPassword("hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	if !auth.CheckPassword(hash, "hunter2") {
		t.Error("CheckPassword should return true for correct password")
	}
	if auth.CheckPassword(hash, "wrong") {
		t.Error("CheckPassword should return false for wrong password")
	}
}

// ── token ─────────────────────────────────────────────────────────────────────

func TestTokenRoundtrip(t *testing.T) {
	cfg := testTokenCfg()
	tokenStr, err := auth.NewToken(cfg, "u1", "alice", "admin")
	if err != nil {
		t.Fatal(err)
	}

	claims, err := auth.ParseToken(cfg, tokenStr)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "u1" || claims.Username != "alice" || claims.Role != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestTokenWrongSecret(t *testing.T) {
	cfg := testTokenCfg()
	tokenStr, err := auth.NewToken(cfg, "u1", "alice", "user")
	if err != nil {
		t.Fatal(err)
	}

	wrongCfg := auth.TokenConfig{Secret: []byte("different-secret"), Expiry: time.Hour}
	if _, err := auth.ParseToken(wrongCfg, tokenStr); err == nil {
		t.Fatal("expected error for token verified with wrong secret")
	}
}

func TestTokenExpired(t *testing.T) {
	cfg := testTokenCfg()
	past := time.Now().Add(-time.Hour)
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(past),
			IssuedAt:  jwt.NewNumericDate(past.Add(-time.Hour)),
		},
		UserID:   "u1",
		Username: "alice",
		Role:     "user",
	}
	tokenStr, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(cfg.Secret)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := auth.ParseToken(cfg, tokenStr); err == nil {
		t.Fatal("expected error for expired token")
	}
}

// ── middleware ────────────────────────────────────────────────────────────────

func TestAuthenticateMiddleware(t *testing.T) {
	cfg := testTokenCfg()

	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		claims, ok := auth.ClaimsFromContext(r.Context())
		if !ok {
			t.Error("expected claims in context")
		}
		if claims.UserID != "u42" {
			t.Errorf("unexpected UserID in claims: %s", claims.UserID)
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := auth.Authenticate(cfg)(inner)

	t.Run("valid token", func(t *testing.T) {
		reached = false
		tokenStr, _ := auth.NewToken(cfg, "u42", "bob", "user")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: tokenStr})

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if !reached {
			t.Error("inner handler was not called")
		}
	})

	t.Run("missing cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("malformed token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: "not.a.token"})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})
}

func TestRequireAdmin(t *testing.T) {
	cfg := testTokenCfg()
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := auth.Authenticate(cfg)(auth.RequireAdmin(inner))

	makeReq := func(role string) *http.Request {
		tokenStr, _ := auth.NewToken(cfg, "u1", "alice", role)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: tokenStr})
		return req
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, makeReq("admin"))
	if rr.Code != http.StatusOK {
		t.Errorf("admin: expected 200, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, makeReq("user"))
	if rr.Code != http.StatusForbidden {
		t.Errorf("user: expected 403, got %d", rr.Code)
	}
}

// ── handler: Login (local auth) ───────────────────────────────────────────────

func TestLoginLocalOK(t *testing.T) {
	d := openTestDB(t)
	seedUser(t, d, "alice", "password123", "admin")

	h := auth.NewHandler(d, testTokenCfg(), nil) // nil = local auth
	body := `{"username":"alice","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}

	if rr.Result().Cookies()[0].Name != auth.AuthCookieName {
		t.Error("expected auth cookie in response")
	}
	var resp struct{ User auth.UserDTO }
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.Username != "alice" || resp.User.Role != "admin" {
		t.Errorf("unexpected user in response: %+v", resp.User)
	}
}

func TestLoginLocalWrongPassword(t *testing.T) {
	d := openTestDB(t)
	seedUser(t, d, "alice", "password123", "user")

	h := auth.NewHandler(d, testTokenCfg(), nil)
	body := `{"username":"alice","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestLoginLocalUnknownUser(t *testing.T) {
	d := openTestDB(t)
	h := auth.NewHandler(d, testTokenCfg(), nil)

	body := `{"username":"ghost","password":"pw"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestLoginMissingFields(t *testing.T) {
	d := openTestDB(t)
	h := auth.NewHandler(d, testTokenCfg(), nil)

	for _, body := range []string{
		`{"username":"alice"}`,
		`{"password":"pw"}`,
		`{}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %q: expected 400, got %d", body, rr.Code)
		}
	}
}

func TestLoginBadJSON(t *testing.T) {
	d := openTestDB(t)
	h := auth.NewHandler(d, testTokenCfg(), nil)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ── handler: Login (ABS auth) ─────────────────────────────────────────────────

func TestLoginABSOK(t *testing.T) {
	d := openTestDB(t)
	stub := absStub{user: &abs.User{ID: "abs-u1", Username: "alice", Type: "admin"}}
	h := auth.NewHandler(d, testTokenCfg(), stub)

	body := `{"username":"alice","password":"anypassword"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}

	if rr.Result().Cookies()[0].Name != auth.AuthCookieName {
		t.Error("expected auth cookie in response")
	}
	var resp struct{ User auth.UserDTO }
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.User.ID != "abs-u1" || resp.User.Username != "alice" || resp.User.Role != "admin" {
		t.Errorf("unexpected user in response: %+v", resp.User)
	}
}

func TestLoginABSInvalidCredentials(t *testing.T) {
	d := openTestDB(t)
	stub := absStub{err: abs.ErrInvalidCredentials}
	h := auth.NewHandler(d, testTokenCfg(), stub)

	body := `{"username":"alice","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestLoginABSUpsertsPersistsUser(t *testing.T) {
	d := openTestDB(t)
	stub := absStub{user: &abs.User{ID: "abs-u2", Username: "bob", Type: "user"}}
	h := auth.NewHandler(d, testTokenCfg(), stub)

	body := `{"username":"bob","password":"pw"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Verify the user landed in the DB so subsequent Me/request queries work.
	user, err := d.GetUserByID(context.Background(), "abs-u2")
	if err != nil {
		t.Fatalf("expected user in DB after ABS login: %v", err)
	}
	if user.Username != "bob" || user.Role != "user" {
		t.Errorf("unexpected persisted user: %+v", user)
	}
}

// ── handler: Me ───────────────────────────────────────────────────────────────

func TestMe(t *testing.T) {
	d := openTestDB(t)
	id := seedUser(t, d, "bob", "pw", "user")
	cfg := testTokenCfg()

	tokenStr, err := auth.NewToken(cfg, id, "bob", "user")
	if err != nil {
		t.Fatal(err)
	}

	h := auth.NewHandler(d, cfg, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.AuthCookieName, Value: tokenStr})

	rr := httptest.NewRecorder()
	auth.Authenticate(cfg)(http.HandlerFunc(h.Me)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var dto auth.UserDTO
	if err := json.NewDecoder(rr.Body).Decode(&dto); err != nil {
		t.Fatal(err)
	}
	if dto.Username != "bob" || dto.ID != id {
		t.Errorf("unexpected dto: %+v", dto)
	}
}
