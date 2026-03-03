// Package abs provides a minimal AudioBookShelf API client used for
// delegating user authentication to an existing ABS instance.
package abs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ErrInvalidCredentials is returned when ABS rejects the login credentials.
var ErrInvalidCredentials = errors.New("invalid credentials")

// User represents the ABS user returned on a successful login.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	// Type is "root", "admin", or "user" in ABS terminology.
	Type string `json:"type"`
}

// Role maps the ABS user type to a Bookarr role string.
func (u *User) Role() string {
	if u.Type == "root" || u.Type == "admin" {
		return "admin"
	}
	return "user"
}

// Client is a minimal ABS API client scoped to authentication.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a Client pointing at baseURL (trailing slashes stripped).
func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type absLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type absLoginResponse struct {
	User User `json:"user"`
}

// Login authenticates username/password against ABS POST /api/login.
// Returns ErrInvalidCredentials if ABS rejects the credentials (401/403).
// Any other non-200 response is returned as a generic error.
func (c *Client) Login(ctx context.Context, username, password string) (*User, error) {
	payload, err := json.Marshal(absLoginRequest{Username: username, Password: password})
	if err != nil {
		return nil, fmt.Errorf("marshal abs login request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/login", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build abs login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("abs login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrInvalidCredentials
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("abs login: unexpected status %d", resp.StatusCode)
	}

	var result absLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode abs login response: %w", err)
	}
	if result.User.ID == "" {
		return nil, fmt.Errorf("abs login: empty user ID in response")
	}
	return &result.User, nil
}
