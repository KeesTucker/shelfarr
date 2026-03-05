// Package qbit provides an HTTP client for the qBittorrent Web API.
// It manages session-cookie authentication automatically, re-logging in when
// the session expires.
package qbit

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrNotFound is returned when a requested torrent is not present in qBittorrent.
var ErrNotFound = errors.New("qbit: torrent not found")

// appPreferences is the subset of /api/v2/app/preferences we care about.
type appPreferences struct {
	SavePath string `json:"save_path"`
}

// TorrentInfo is a subset of the fields returned by the /api/v2/torrents/info
// endpoint. Progress runs from 0.0 (not started) to 1.0 (complete).
type TorrentInfo struct {
	Hash     string  `json:"hash"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	State    string  `json:"state"` // e.g. "downloading", "seeding", "error"
	SavePath string  `json:"save_path"`
	AddedOn  int64   `json:"added_on"` // unix timestamp
}

// Client is an HTTP client for the qBittorrent Web API.
// It is safe for concurrent use; session re-login is handled internally.
type Client struct {
	baseURL  string
	username string
	password string
	autoTMM  bool
	http     *http.Client

	mu     sync.Mutex
	cookie *http.Cookie // SID session cookie
}

// SetAutoTMM enables Automatic Torrent Management. When enabled, torrents are
// added with autoTMM=true and qBittorrent uses its category-based save path
// rules, ignoring any savepath passed to AddTorrent.
func (c *Client) SetAutoTMM(enabled bool) {
	c.autoTMM = enabled
}

// AutoTMM reports whether Automatic Torrent Management is enabled.
func (c *Client) AutoTMM() bool {
	return c.autoTMM
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
// save path and category, returning the torrent infohash. category may be
// empty to leave the torrent uncategorised.
//
// Magnet URIs: the infohash is parsed directly and the URI is posted to qBit.
//
// HTTP torrent URLs: our backend fetches the .torrent bytes (the Prowlarr
// proxy URL carries the API key, so private-tracker auth is handled here, not
// by qBittorrent), then uploads the file directly via multipart. qBit is then
// polled briefly to discover the assigned infohash.
func (c *Client) AddTorrent(ctx context.Context, downloadURL, savePath, category string) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("qbit: QBIT_URL is not configured")
	}

	if strings.HasPrefix(downloadURL, "magnet:") {
		hash, ok := parseMagnetHash(downloadURL)
		if !ok {
			return "", fmt.Errorf("qbit: could not parse infohash from magnet URI")
		}
		if err := c.postTorrent(ctx, downloadURL, savePath, category); err != nil {
			return "", err
		}
		return hash, nil
	}

	// HTTP torrent URL: download the file ourselves and upload to qBit so that
	// private-tracker authentication is handled by our backend.
	addedAfter := time.Now().Unix()
	if err := c.fetchAndPostTorrent(ctx, downloadURL, savePath, category); err != nil {
		return "", err
	}
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
	defer func() { _ = resp.Body.Close() }()

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

// SetCategory changes the category of a torrent identified by its infohash.
// Passing an empty string removes the torrent from any category.
// If the category does not yet exist in qBittorrent (409 Conflict), it is
// created automatically and the call is retried once.
func (c *Client) SetCategory(ctx context.Context, hash, category string) error {
	if c.baseURL == "" {
		return fmt.Errorf("qbit: QBIT_URL is not configured")
	}

	makeReq := func() (*http.Request, error) {
		form := url.Values{}
		form.Set("hashes", hash)
		form.Set("category", category)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/api/v2/torrents/setCategory",
			strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	}

	resp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return fmt.Errorf("qbit set category: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusConflict {
		_, _ = io.Copy(io.Discard, resp.Body)
		// Category does not exist yet — create it and retry.
		if err := c.createCategory(ctx, category); err != nil {
			return fmt.Errorf("qbit set category: auto-create category: %w", err)
		}
		resp2, err := c.doWithAuth(ctx, makeReq)
		if err != nil {
			return fmt.Errorf("qbit set category (retry): %w", err)
		}
		defer func() { _ = resp2.Body.Close() }()
		if resp2.StatusCode != http.StatusOK {
			return fmt.Errorf("qbit set category (retry): unexpected status %d", resp2.StatusCode)
		}
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbit set category: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// createCategory creates a new category in qBittorrent with no save-path override.
func (c *Client) createCategory(ctx context.Context, category string) error {
	makeReq := func() (*http.Request, error) {
		form := url.Values{}
		form.Set("category", category)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/api/v2/torrents/createCategory",
			strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	}

	resp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection is returned to the pool
	return nil
}

// RemoveTorrent removes a torrent from qBittorrent by infohash without
// deleting the downloaded files. The qBittorrent delete endpoint is
// idempotent — passing an unknown hash returns 200.
func (c *Client) RemoveTorrent(ctx context.Context, hash string) error {
	if c.baseURL == "" {
		return fmt.Errorf("qbit: QBIT_URL is not configured")
	}

	makeReq := func() (*http.Request, error) {
		form := url.Values{}
		form.Set("hashes", hash)
		form.Set("deleteFiles", "false")
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/api/v2/torrents/delete",
			strings.NewReader(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	}

	resp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return fmt.Errorf("qbit remove torrent: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbit remove torrent: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// GetDefaultSavePath returns the default download directory configured in
// qBittorrent (Settings → Downloads → Default Save Path).
func (c *Client) GetDefaultSavePath(ctx context.Context) (string, error) {
	if c.baseURL == "" {
		return "", fmt.Errorf("qbit: QBIT_URL is not configured")
	}

	makeReq := func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet,
			c.baseURL+"/api/v2/app/preferences", nil)
	}

	resp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return "", fmt.Errorf("qbit get preferences: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("qbit get preferences: unexpected status %d", resp.StatusCode)
	}

	var prefs appPreferences
	if err := json.NewDecoder(resp.Body).Decode(&prefs); err != nil {
		return "", fmt.Errorf("qbit get preferences: decode: %w", err)
	}
	if prefs.SavePath == "" {
		return "", fmt.Errorf("qbit get preferences: save_path is empty")
	}
	return prefs.SavePath, nil
}

// ── internal helpers ──────────────────────────────────────────────────────────

// postTorrent POSTs a magnet URI to qBittorrent's add endpoint.
func (c *Client) postTorrent(ctx context.Context, downloadURL, savePath, category string) error {
	makeReq := func() (*http.Request, error) {
		form := url.Values{}
		form.Set("urls", downloadURL)
		if c.autoTMM {
			form.Set("autoTMM", "true")
		} else {
			form.Set("savepath", savePath)
		}
		if category != "" {
			form.Set("category", category)
		}
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
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "Ok." {
		return fmt.Errorf("qbit add torrent: unexpected response %d %q", resp.StatusCode, string(body))
	}
	return nil
}

// fetchAndPostTorrent downloads the .torrent file at downloadURL using our own
// HTTP client, then uploads the bytes to qBittorrent via multipart form. This
// handles private-tracker URLs (e.g. Prowlarr proxy links) that embed API-key
// auth in the URL — qBittorrent itself never needs to reach the tracker.
func (c *Client) fetchAndPostTorrent(ctx context.Context, downloadURL, savePath, category string) error {
	// Step 1: fetch the .torrent bytes.
	fetchReq, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("qbit fetch torrent: build request: %w", err)
	}
	fetchResp, err := c.http.Do(fetchReq) //nolint:gosec
	if err != nil {
		return fmt.Errorf("qbit fetch torrent: %w", err)
	}
	defer func() { _ = fetchResp.Body.Close() }()
	if fetchResp.StatusCode != http.StatusOK {
		return fmt.Errorf("qbit fetch torrent: status %d", fetchResp.StatusCode)
	}
	torrentBytes, err := io.ReadAll(fetchResp.Body)
	if err != nil {
		return fmt.Errorf("qbit fetch torrent: read body: %w", err)
	}

	// Step 2: upload to qBit as a multipart file.
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("torrents", "upload.torrent")
	if err != nil {
		return fmt.Errorf("qbit upload torrent: create form file: %w", err)
	}
	if _, err := fw.Write(torrentBytes); err != nil {
		return fmt.Errorf("qbit upload torrent: write bytes: %w", err)
	}
	if c.autoTMM {
		if err := mw.WriteField("autoTMM", "true"); err != nil {
			return fmt.Errorf("qbit upload torrent: write autoTMM: %w", err)
		}
	} else {
		if err := mw.WriteField("savepath", savePath); err != nil {
			return fmt.Errorf("qbit upload torrent: write savepath: %w", err)
		}
	}
	if category != "" {
		if err := mw.WriteField("category", category); err != nil {
			return fmt.Errorf("qbit upload torrent: write category: %w", err)
		}
	}
	if err := mw.Close(); err != nil {
		return fmt.Errorf("qbit upload torrent: close multipart writer: %w", err)
	}

	ct := mw.FormDataContentType()
	makeReq := func() (*http.Request, error) {
		r, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/api/v2/torrents/add",
			bytes.NewReader(buf.Bytes()))
		if err != nil {
			return nil, err
		}
		r.Header.Set("Content-Type", ct)
		return r, nil
	}

	addResp, err := c.doWithAuth(ctx, makeReq)
	if err != nil {
		return fmt.Errorf("qbit upload torrent: %w", err)
	}
	defer func() { _ = addResp.Body.Close() }()
	body, _ := io.ReadAll(addResp.Body)
	if addResp.StatusCode != http.StatusOK || strings.TrimSpace(string(body)) != "Ok." {
		return fmt.Errorf("qbit upload torrent: unexpected response %d %q", addResp.StatusCode, string(body))
	}
	return nil
}

// findRecentlyAdded polls the full torrent list and returns the hash of the
// first torrent whose added_on timestamp is >= addedAfter. Retries up to 5
// times with 1-second pauses, respecting context cancellation.
func (c *Client) findRecentlyAdded(ctx context.Context, addedAfter int64) (string, error) {
	for attempt := 0; attempt < 15; attempt++ {
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
	return "", fmt.Errorf("qbit: newly added torrent not visible after 15 attempts")
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
	defer func() { _ = resp.Body.Close() }()

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

	resp, err := c.http.Do(req) //nolint:gosec
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusForbidden {
		_ = resp.Body.Close()
		// Session expired — re-login and retry once.
		if err := c.login(ctx); err != nil {
			return nil, err
		}
		req, err = makeReq()
		if err != nil {
			return nil, err
		}
		c.injectCookie(req)
		return c.http.Do(req) //nolint:gosec
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

	resp, err := c.http.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("qbit login: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

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
