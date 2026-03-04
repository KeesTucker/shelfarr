package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"shelfarr/internal/abs"
	"shelfarr/internal/db"
	"shelfarr/internal/respond"
)

// absAuthenticator is the interface used to delegate login to AudioBookShelf.
// *abs.Client satisfies it. Pass nil to fall back to local bcrypt auth.
type absAuthenticator interface {
	Login(ctx context.Context, username, password string) (*abs.User, error)
}

// Handler handles the /api/auth/* routes.
type Handler struct {
	db      *db.DB
	cfg     TokenConfig
	absAuth absAuthenticator // nil → local DB auth
}

// NewHandler creates an auth handler. When absAuth is non-nil, Login delegates
// credential validation to AudioBookShelf; otherwise local bcrypt is used.
func NewHandler(database *db.DB, cfg TokenConfig, absAuth absAuthenticator) *Handler {
	return &Handler{db: database, cfg: cfg, absAuth: absAuth}
}

// ── request / response types ──────────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` //nolint:gosec
}

// AuthCookieName is the name of the httpOnly session cookie.
const AuthCookieName = "shelfarr_token" //nolint:gosec

const authCookieName = AuthCookieName

type loginResponse struct {
	User UserDTO `json:"user"`
}

// UserDTO is the public representation of a user returned in API responses.
type UserDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// ── handlers ──────────────────────────────────────────────────────────────────

// Login validates credentials and returns a signed JWT.
// When an ABS client is configured it proxies to ABS; otherwise it checks
// the local bcrypt-hashed password in the database.
//
//	POST /api/auth/login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		respond.Error(w, http.StatusBadRequest, "username and password required")
		return
	}

	if h.absAuth != nil {
		h.loginABS(w, r, req)
		return
	}
	h.loginLocal(w, r, req)
}

func (h *Handler) loginABS(w http.ResponseWriter, r *http.Request, req loginRequest) {
	absUser, err := h.absAuth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		if errors.Is(err, abs.ErrInvalidCredentials) {
			respond.Error(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		slog.Error("abs login failed", "err", err)
		respond.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if err := h.db.UpsertABSUser(r.Context(), absUser.ID, absUser.Username, absUser.Role()); err != nil {
		slog.Error("upsert abs user", "err", err)
		respond.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	token, err := NewToken(h.cfg, absUser.ID, absUser.Username, absUser.Role())
	if err != nil {
		slog.Error("sign JWT for abs user", "err", err)
		respond.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.setAuthCookie(w, token)
	respond.JSON(w, http.StatusOK, loginResponse{
		User: UserDTO{ID: absUser.ID, Username: absUser.Username, Role: absUser.Role()},
	})
}

func (h *Handler) loginLocal(w http.ResponseWriter, r *http.Request, req loginRequest) {
	user, err := h.db.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		respond.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !CheckPassword(user.PasswordHash, req.Password) {
		respond.Error(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := NewToken(h.cfg, user.ID, user.Username, user.Role)
	if err != nil {
		slog.Error("failed to sign JWT", "err", err)
		respond.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.setAuthCookie(w, token)
	respond.JSON(w, http.StatusOK, loginResponse{
		User: UserDTO{ID: user.ID, Username: user.Username, Role: user.Role},
	})
}

// Logout clears the auth cookie.
//
//	POST /api/auth/logout
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		HttpOnly: true,
		Secure:   h.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode,
		Path:     "/",
		MaxAge:   int(h.cfg.Expiry.Seconds()),
	})
}

// Me returns the current user's profile. The DB is consulted so the response
// reflects any role changes made after the token was issued.
//
//	GET /api/auth/me
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		respond.Error(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.db.GetUserByID(r.Context(), claims.UserID)
	if err != nil {
		respond.Error(w, http.StatusUnauthorized, "user not found")
		return
	}

	respond.JSON(w, http.StatusOK, UserDTO{
		ID:       user.ID,
		Username: user.Username,
		Role:     user.Role,
	})
}
