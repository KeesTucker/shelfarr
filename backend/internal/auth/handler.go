package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"bookarr/internal/db"
	"bookarr/internal/respond"
)

// Handler handles the /api/auth/* routes.
type Handler struct {
	db  *db.DB
	cfg TokenConfig
}

// NewHandler creates an auth handler bound to the given DB and token config.
func NewHandler(database *db.DB, cfg TokenConfig) *Handler {
	return &Handler{db: database, cfg: cfg}
}

// ── request / response types ──────────────────────────────────────────────────

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string  `json:"token"`
	User  UserDTO `json:"user"`
}

// UserDTO is the public representation of a user returned in API responses.
type UserDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// ── handlers ──────────────────────────────────────────────────────────────────

// Login validates the provided credentials and returns a signed JWT.
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

	user, err := h.db.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		// Return the same generic message for "no such user" and "wrong
		// password" to prevent username enumeration.
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

	respond.JSON(w, http.StatusOK, loginResponse{
		Token: token,
		User:  UserDTO{ID: user.ID, Username: user.Username, Role: user.Role},
	})
}

// createUserRequest is the body accepted by CreateUser.
type createUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateUser provisions a new regular user account. Intended for Wizarr (and
// similar provisioning tools) that call this endpoint with a shared service
// token rather than a user JWT.
//
//	POST /api/users
func (h *Handler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respond.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		respond.Error(w, http.StatusBadRequest, "username and password required")
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		slog.Error("hash password for new user", "err", err)
		respond.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	id := uuid.NewString()
	if err := h.db.CreateUser(r.Context(), id, req.Username, hash, "user"); err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			respond.Error(w, http.StatusConflict, "username already taken")
			return
		}
		slog.Error("create user", "err", err)
		respond.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	slog.Info("user created via service token", "username", req.Username, "id", id)
	respond.JSON(w, http.StatusCreated, UserDTO{ID: id, Username: req.Username, Role: "user"})
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
