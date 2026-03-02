// Package qbit provides an HTTP client for the qBittorrent Web API.
// It manages session-cookie authentication automatically, re-logging in when
// the session expires.
package qbit

import (
	"context"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when a requested torrent is not present in qBittorrent.
var ErrNotFound = errors.New("qbit: torrent not found")

// TorrentInfo is a subset of the fields returned by the /api/v2/torrents/info
// endpoint. Progress runs from 0.0 (not started) to 1.0 (complete).
type TorrentInfo struct {
	Hash     string  `json:"hash"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	State    string  `json:"state"`    // e.g. "downloading", "seeding", "error"
	SavePath string  `json:"save_path"`
	AddedOn  int64   `json:"added_on"` // unix timestamp
}

// Client is an HTTP client for the qBittorrent Web API.
// It is safe for concurrent use; session re-login is handled internally.
type Client struct {
	baseURL  string
	username string
	password string
	http     *http.Client

	mu     sync.Mutex
	cookie *http.Cookie // SID session cookie
}

// New creates a qBittorrent client. baseURL must not have a trailing slash.
func New(baseURL, username, password string) *Client {
	return &Client{
		baseURL:  baseURL,
		username: username,
		password: password,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// AddTorrent sends a magnet URI or torrent URL to qBittorrent with the given
// save path and returns the torrent infohash. For magnet URIs the hash is
// parsed directly from the URI; for HTTP torrent URLs qBittorrent is polled
// briefly (up to 5 s) to discover the hash assigned after download.
func (c *Client) AddTorrent(ctx context.Context, downloadURL, savePath string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("qbit: QBIT_URL is not configured")
	}

	isMagnet := strings.HasPrefix(downloadURL, "magnet:")

	var magnetInfoHash string
	if isMagnet {
		var ok bool
		magnetInfoHash, ok = parseMagnetHash(downloadURL)
		if !ok {
			return "", fmt.Errorf("qbit: could not parse infohash from magnet URI")
		}
	}

	addedAfter := time.Now().Unix()

	if err := c.postTorrent(ctx, downloadURL, savePath); err != nil {
		return "", err
	}

	if isMagnet {
		return magnetInfoHash, nil
	}

	// For HTTP torrent URLs qBittorrent downloads the .torrent file
	// asynchronously, so we poll until the new entry appears.
	return c.findRecentlyAdded(ctx, addedAfter)
}

// GetTorrent retrieves info for a single torrent by its infohash.
// Returns ErrNotFound if qBittorrent has no torrent with that hash.
func (c *Client) GetTorrent(ctx context.Context, hash string) (*TorrentInfo, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("qbit: QBIT_URL is not configured")
	}

	makeReq := func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			c.baseURL+"/api/v2/torrents/info", nil)
		if err != nil {
			return nil, err
		}
		q := req.URL.Query()
		q.Set("hashes", hash)
		req.URL.RawQuery = q.Encode()
		return req, nil
	}

	resp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return nil, fmt.Errorf("qbit get torrent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("qbit get torrent: unexpected status %d", resp.StatusCode)
	}

	var torrents []TorrentInfo
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("qbit get torrent: decode: %w", err)
	}
	if len(torrents) == 0 {
		return nil, ErrNotFound
	}
	return &torrents[0], nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// postTorrent POSTs a magnet URI or torrent URL to qBittorrent's add endpoint.
func (c *Client) postTorrent(ctx context.Context, downloadURL, savePath string) error {
	makeReq := func() (*http.Request, error) {
		form := url.Values{}
		form.Set("urls", downloadURL)
		form.Set("savepath", savePath)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/api/v2/torrents/add",
			strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	}

	resp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return fmt.Errorf("qbit add torrent: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "Ok." {
		return fmt.Errorf("qbit add torrent: unexpected response %d %q", resp.StatusCode, string(body))
	}
	return nil
}

// findRecentlyAdded polls the full torrent list and returns the hash of the
// first torrent whose added_on timestamp is >= addedAfter. Retries up to 5
// times with 1-second pauses, respecting context cancellation.
func (c *Client) findRecentlyAdded(ctx context.Context, addedAfter int64) (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Second):
		}

		torrents, err := c.listAll(ctx)
		if err != nil {
			return "", err
		}
		for _, t := range torrents {
			if t.AddedOn >= addedAfter {
				return t.Hash, nil
			}
		}
	}
	return "", fmt.Errorf("qbit: newly added torrent not visible after 5 attempts")
}

// listAll returns all torrents currently tracked by qBittorrent.
func (c *Client) listAll(ctx context.Context) ([]TorrentInfo, error) {
	makeReq := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet,
			c.baseURL+"/api/v2/torrents/info", nil)
	}

	resp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return nil, fmt.Errorf("qbit list torrents: %w", err)
	}
	defer resp.Body.Close()

	var torrents []TorrentInfo
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, fmt.Errorf("qbit list torrents: decode: %w", err)
	}
	return torrents, nil
}

// doWithAuth executes an HTTP request with session-cookie authentication.
// On HTTP 403 it re-authenticates and retries once.
func (c *Client) doWithAuth(ctx context.Context, makeReq func() (*http.Request, error)) (*http.Response, error) {
	if err := c.ensureLoggedIn(ctx); err != nil {
		return nil, err
	}

	req, err := makeReq()
	if err != nil {
		return nil, err
	}
	c.injectCookie(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		// Session expired — re-login and retry once.
		if err := c.login(ctx); err != nil {
			return nil, err
		}
		req, err = makeReq()
		if err != nil {
			return nil, err
		}
		c.injectCookie(req)
		return c.http.Do(req)
	}

	return resp, nil
}

func (c *Client) injectCookie(req *http.Request) {
	c.mu.Lock()
	cookie := c.cookie
	c.mu.Unlock()
	if cookie != nil {
		req.AddCookie(cookie)
	}
}

func (c *Client) ensureLoggedIn(ctx context.Context) error {
	c.mu.Lock()
	hasSession := c.cookie != nil
	c.mu.Unlock()
	if hasSession {
		return nil
	}
	return c.login(ctx)
}

// login authenticates with qBittorrent and stores the SID session cookie.
func (c *Client) login(ctx context.Context) error {
	form := url.Values{}
	form.Set("username", c.username)
	form.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v2/auth/login",
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("qbit login: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("qbit login: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "Ok." {
		return fmt.Errorf("qbit login: authentication failed (check credentials), got %q", string(body))
	}

	c.mu.Lock()
	for _, ck := range resp.Cookies() {
		if ck.Name == "SID" {
			c.cookie = ck
			break
		}
	}
	c.mu.Unlock()
	return nil
}

// parseMagnetHash extracts the lowercase hex infohash from a magnet URI.
// Handles both hex-encoded (40 chars) and base32-encoded (32 chars) hashes.
func parseMagnetHash(magnetURI string) (string, bool) {
	u, err := url.Parse(magnetURI)
	if err != nil {
		return "", false
	}
	xt := u.Query().Get("xt")
	hash, found := strings.CutPrefix(xt, "urn:btih:")
	if !found {
		return "", false
	}
	hash = strings.ToLower(hash)

	// Base32-encoded hashes are 32 chars; hex are 40. Convert if needed.
	if len(hash) == 32 {
		decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).
			DecodeString(strings.ToUpper(hash))
		if err != nil {
			// Return as-is — qBit may still handle it.
			return hash, true
		}
		hash = hex.EncodeToString(decoded)
	}
	return hash, true
}
