package abs_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"shelfarr/internal/abs"
)

// fakeABS builds a test server that handles POST /api/login.
// respondWith controls what the fake ABS returns.
func fakeABS(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", handler)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func absLoginBody(user abs.User) []byte {
	type response struct {
		User abs.User `json:"user"`
	}
	b, _ := json.Marshal(response{User: user})
	return b
}

// ── Login success cases ───────────────────────────────────────────────────────

func TestLoginSuccess(t *testing.T) {
	cases := []struct {
		name     string
		absType  string
		wantRole string
	}{
		{"root user maps to admin role", "root", "admin"},
		{"admin user maps to admin role", "admin", "admin"},
		{"regular user maps to user role", "user", "user"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := fakeABS(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.Write(absLoginBody(abs.User{
					ID:       "abs-123",
					Username: "alice",
					Type:     tc.absType,
				}))
			})

			client := abs.New(srv.URL)
			user, err := client.Login(context.Background(), "alice", "pw")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if user.ID != "abs-123" {
				t.Errorf("ID: want abs-123, got %q", user.ID)
			}
			if user.Username != "alice" {
				t.Errorf("Username: want alice, got %q", user.Username)
			}
			if user.Role() != tc.wantRole {
				t.Errorf("Role(): want %q, got %q", tc.wantRole, user.Role())
			}
		})
	}
}

// ── Login failure cases ───────────────────────────────────────────────────────

func TestLoginUnauthorized(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			srv := fakeABS(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			})

			client := abs.New(srv.URL)
			_, err := client.Login(context.Background(), "alice", "wrong")
			if !errors.Is(err, abs.ErrInvalidCredentials) {
				t.Errorf("want ErrInvalidCredentials, got %v", err)
			}
		})
	}
}

func TestLoginUnexpectedStatus(t *testing.T) {
	srv := fakeABS(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := abs.New(srv.URL)
	_, err := client.Login(context.Background(), "alice", "pw")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if errors.Is(err, abs.ErrInvalidCredentials) {
		t.Error("should not be ErrInvalidCredentials for 500")
	}
}

func TestLoginEmptyUserID(t *testing.T) {
	srv := fakeABS(t, func(w http.ResponseWriter, _ *http.Request) {
		// Returns 200 but with an empty user object.
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"user":{}}`))
	})

	client := abs.New(srv.URL)
	_, err := client.Login(context.Background(), "alice", "pw")
	if err == nil {
		t.Fatal("expected error for empty user ID")
	}
}

func TestLoginBadJSON(t *testing.T) {
	srv := fakeABS(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	})

	client := abs.New(srv.URL)
	_, err := client.Login(context.Background(), "alice", "pw")
	if err == nil {
		t.Fatal("expected error for invalid JSON response")
	}
}

func TestLoginForwardsCredentials(t *testing.T) {
	var gotUsername, gotPassword string

	srv := fakeABS(t, func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotUsername = body.Username
		gotPassword = body.Password

		w.Header().Set("Content-Type", "application/json")
		w.Write(absLoginBody(abs.User{ID: "u1", Username: body.Username, Type: "user"}))
	})

	client := abs.New(srv.URL)
	_, err := client.Login(context.Background(), "testuser", "secret123")
	if err != nil {
		t.Fatal(err)
	}
	if gotUsername != "testuser" {
		t.Errorf("username: want testuser, got %q", gotUsername)
	}
	if gotPassword != "secret123" {
		t.Errorf("password: want secret123, got %q", gotPassword)
	}
}

func TestLoginTrailingSlashInBaseURL(t *testing.T) {
	srv := fakeABS(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(absLoginBody(abs.User{ID: "u1", Username: "alice", Type: "user"}))
	})

	// Trailing slash must be stripped; /api/login should still be reached.
	client := abs.New(srv.URL + "/")
	_, err := client.Login(context.Background(), "alice", "pw")
	if err != nil {
		t.Fatalf("unexpected error with trailing slash in base URL: %v", err)
	}
}
