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

	"bookarr/internal/auth"
	"bookarr/internal/db"
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
	// Manually craft a token with an expiry in the past.
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

	// The inner handler just records that it was reached.
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
		req.Header.Set("Authorization", "Bearer "+tokenStr)

		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rr.Code)
		}
		if !reached {
			t.Error("inner handler was not called")
		}
	})

	t.Run("missing header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("malformed token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer not.a.token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rr.Code)
		}
	})

	t.Run("wrong prefix", func(t *testing.T) {
		tokenStr, _ := auth.NewToken(cfg, "u42", "bob", "user")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Token "+tokenStr) // not "Bearer"
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
		req.Header.Set("Authorization", "Bearer "+tokenStr)
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

// ── handler: Login ────────────────────────────────────────────────────────────

func TestLoginOK(t *testing.T) {
	d := openTestDB(t)
	seedUser(t, d, "alice", "password123", "admin")

	h := auth.NewHandler(d, testTokenCfg())
	body := `{"username":"alice","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var resp struct {
		Token string       `json:"token"`
		User  auth.UserDTO `json:"user"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Error("expected non-empty token")
	}
	if resp.User.Username != "alice" || resp.User.Role != "admin" {
		t.Errorf("unexpected user in response: %+v", resp.User)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	d := openTestDB(t)
	seedUser(t, d, "alice", "password123", "user")

	h := auth.NewHandler(d, testTokenCfg())
	body := `{"username":"alice","password":"wrong"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestLoginUnknownUser(t *testing.T) {
	d := openTestDB(t)
	h := auth.NewHandler(d, testTokenCfg())

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
	h := auth.NewHandler(d, testTokenCfg())

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
	h := auth.NewHandler(d, testTokenCfg())

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("not json"))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ── middleware: RequireServiceToken ───────────────────────────────────────────

func TestRequireServiceToken(t *testing.T) {
	const token = "super-secret-service-token"

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	cases := []struct {
		name       string
		configured string // token passed to RequireServiceToken
		header     string // Authorization header value sent by client
		wantStatus int
	}{
		{
			name:       "valid token passes through",
			configured: token,
			header:     "Bearer " + token,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing header returns 403",
			configured: token,
			header:     "",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "wrong token returns 403",
			configured: token,
			header:     "Bearer wrong-token",
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "wrong scheme returns 403",
			configured: token,
			header:     "Token " + token, // not Bearer
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "unconfigured service token returns 503",
			configured: "", // empty = SERVICE_TOKEN not set
			header:     "Bearer anything",
			wantStatus: http.StatusServiceUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := auth.RequireServiceToken(tc.configured)(inner)
			req := httptest.NewRequest(http.MethodPost, "/api/users", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != tc.wantStatus {
				t.Errorf("expected %d, got %d", tc.wantStatus, rr.Code)
			}
		})
	}
}

// ── handler: CreateUser ───────────────────────────────────────────────────────

func TestCreateUser(t *testing.T) {
	d := openTestDB(t)
	h := auth.NewHandler(d, testTokenCfg())

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/users", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.CreateUser(rr, req)
		return rr
	}

	t.Run("valid request returns 201 with user DTO", func(t *testing.T) {
		rr := post(`{"username":"wizarr-user","password":"securepass"}`)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d — body: %s", rr.Code, rr.Body)
		}
		var dto auth.UserDTO
		if err := json.NewDecoder(rr.Body).Decode(&dto); err != nil {
			t.Fatal(err)
		}
		if dto.Username != "wizarr-user" {
			t.Errorf("unexpected username: %s", dto.Username)
		}
		if dto.ID == "" {
			t.Error("expected non-empty ID")
		}
		if dto.Role != "user" {
			t.Errorf("expected role 'user', got %q", dto.Role)
		}
	})

	t.Run("created user can log in with the provided password", func(t *testing.T) {
		post(`{"username":"login-verify","password":"mypassword"}`)

		req := httptest.NewRequest(http.MethodPost, "/api/auth/login",
			strings.NewReader(`{"username":"login-verify","password":"mypassword"}`))
		rr := httptest.NewRecorder()
		h.Login(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("login: expected 200, got %d — %s", rr.Code, rr.Body)
		}
	})

	t.Run("duplicate username returns 409", func(t *testing.T) {
		post(`{"username":"dup-user","password":"pw1"}`)
		rr := post(`{"username":"dup-user","password":"pw2"}`)
		if rr.Code != http.StatusConflict {
			t.Errorf("expected 409, got %d — %s", rr.Code, rr.Body)
		}
	})

	t.Run("provisioned user always gets role user", func(t *testing.T) {
		rr := post(`{"username":"role-check","password":"pw123"}`)
		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rr.Code)
		}
		var dto auth.UserDTO
		if err := json.NewDecoder(rr.Body).Decode(&dto); err != nil {
			t.Fatal(err)
		}
		if dto.Role != "user" {
			t.Errorf("expected role 'user', got %q", dto.Role)
		}
	})

	for _, body := range []struct {
		name string
		json string
	}{
		{"missing username", `{"password":"pw"}`},
		{"missing password", `{"username":"nopass"}`},
		{"empty fields", `{}`},
		{"bad JSON", `not json`},
	} {
		t.Run(body.name+" returns 400", func(t *testing.T) {
			rr := post(body.json)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
		})
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

	h := auth.NewHandler(d, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+tokenStr)

	// Run request through the middleware first so claims land in context.
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
