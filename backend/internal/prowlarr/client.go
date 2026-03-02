// Package prowlarr provides an HTTP client for the Prowlarr indexer-aggregator
// API and a chi handler for the /api/search endpoint.
package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Release is a normalised result from Prowlarr's search API.
type Release struct {
	GUID        string
	Title       string
	Size        int64
	Seeders     int
	Indexer     string
	PublishDate time.Time
	// DownloadURL is the URL to add to qBittorrent. It may be a Prowlarr-
	// proxied HTTP URL or a raw magnet URI.
	DownloadURL string
}

type cachedRelease struct {
	release   Release
	expiresAt time.Time
}

// Client is an HTTP client for the Prowlarr API. It is safe for concurrent use.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client

	mu    sync.RWMutex
	cache map[string]cachedRelease // keyed by GUID
}

// New creates a Prowlarr client. baseURL must not have a trailing slash.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 10 * time.Second},
		cache:   make(map[string]cachedRelease),
	}
}

// rawRelease mirrors the JSON shape returned by Prowlarr's /api/v1/search.
type rawRelease struct {
	GUID        string    `json:"guid"`
	Title       string    `json:"title"`
	Size        int64     `json:"size"`
	Seeders     int       `json:"seeders"`
	Leechers    int       `json:"leechers"`
	Indexer     string    `json:"indexer"`
	IndexerID   int       `json:"indexerId"`
	PublishDate time.Time `json:"publishDate"`
	DownloadURL string    `json:"downloadUrl"`
	MagnetURL   string    `json:"magnetUrl"`
}

// Search queries Prowlarr for audiobook releases matching query. Results are
// cached by GUID for 10 minutes to support the request submission flow where
// the client sends back a GUID to fetch the download URL.
func (c *Client) Search(ctx context.Context, query string) ([]Release, error) {
	if c.baseURL == "" {
		return nil, fmt.Errorf("prowlarr: PROWLARR_URL is not configured")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/search", nil)
	if err != nil {
		return nil, fmt.Errorf("prowlarr search: build request: %w", err)
	}

	q := url.Values{}
	q.Set("query", query)
	q.Set("type", "search")
	q.Set("categories", "3030") // 3030 = Audio/Audiobook
	q.Set("apikey", c.apiKey)
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prowlarr search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr search: unexpected status %d", resp.StatusCode)
	}

	var raw []rawRelease
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("prowlarr search: decode response: %w", err)
	}

	results := make([]Release, 0, len(raw))
	expiry := time.Now().Add(10 * time.Minute)

	c.mu.Lock()
	for _, r := range raw {
		dl := r.DownloadURL
		if dl == "" {
			dl = r.MagnetURL
		}
		rel := Release{
			GUID:        r.GUID,
			Title:       r.Title,
			Size:        r.Size,
			Seeders:     r.Seeders,
			Indexer:     r.Indexer,
			PublishDate: r.PublishDate,
			DownloadURL: dl,
		}
		results = append(results, rel)
		if r.GUID != "" {
			c.cache[r.GUID] = cachedRelease{release: rel, expiresAt: expiry}
		}
	}
	c.mu.Unlock()

	return results, nil
}

// GetByGUID retrieves a previously-searched release by its GUID from the
// in-memory cache. Returns false if the GUID is unknown or the cache entry
// has expired. Used by the request submission handler (step 4).
func (c *Client) GetByGUID(guid string) (Release, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cr, ok := c.cache[guid]
	if !ok || time.Now().After(cr.expiresAt) {
		return Release{}, false
	}
	return cr.release, true
}
