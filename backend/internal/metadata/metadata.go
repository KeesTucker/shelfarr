// Package metadata resolves audiobook metadata from OpenLibrary and stores it
// for folder-naming and notification fields on a request.
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultOpenLibraryBase = "https://openlibrary.org"
)

// Book holds the merged audiobook metadata. It is JSON-serialised and stored
// in requests.metadata_json, then used by the file mover for folder naming
// and by Discord for notifications.
type Book struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Year   int    `json:"year,omitempty"`
}

// Client fetches audiobook metadata from OpenLibrary.
type Client struct {
	http            *http.Client
	openLibraryBase string
}

// New returns a Client with a 10-second timeout on all external calls.
func New() *Client {
	return &Client{
		http:            &http.Client{Timeout: 10 * time.Second},
		openLibraryBase: defaultOpenLibraryBase,
	}
}

// Resolve queries OpenLibrary and returns metadata. Source failures are logged
// as warnings but do not return an error — partial results are always preferred
// over nothing. The caller-supplied title and author are used as fallback
// values when the source returns no data.
func (c *Client) Resolve(ctx context.Context, title, author string) *Book {
	ol, err := c.openLibrarySearch(ctx, title, author)
	if err != nil {
		slog.Warn("metadata: OpenLibrary lookup failed", "title", title, "err", err)
	}
	return merge(title, author, ol)
}

// JSON serialises the Book to a JSON string for storage in metadata_json.
func (b *Book) JSON() (string, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("marshal book: %w", err)
	}
	return string(raw), nil
}

// ── merge ─────────────────────────────────────────────────────────────────────

func merge(title, author string, ol *olResult) *Book {
	b := &Book{Title: title, Author: author}
	if ol != nil {
		if ol.title != "" {
			b.Title = ol.title
		}
		if ol.author != "" {
			b.Author = ol.author
		}
		b.Year = ol.year
	}
	return b
}

// ── OpenLibrary ───────────────────────────────────────────────────────────────

type olResult struct {
	title  string
	author string
	year   int
}

// olResponse is the top-level shape returned by /search.json.
type olResponse struct {
	Docs []struct {
		Title            string   `json:"title"`
		AuthorName       []string `json:"author_name"`
		FirstPublishYear int      `json:"first_publish_year"`
	} `json:"docs"`
}

// openLibrarySearch queries https://openlibrary.org/search.json by title and
// author, returning the first matching result. Returns nil (not an error) when
// no results are found.
func (c *Client) openLibrarySearch(ctx context.Context, title, author string) (*olResult, error) {
	q := url.Values{
		"title":  {title},
		"author": {author},
		"fields": {"title,author_name,first_publish_year"},
		"limit":  {"1"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.openLibraryBase+"/search.json?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "shelfarr/1.0")

	resp, err := c.http.Do(req) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var body olResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(body.Docs) == 0 {
		return nil, nil // no results — not an error
	}

	doc := body.Docs[0]
	r := &olResult{title: doc.Title, year: doc.FirstPublishYear}
	if len(doc.AuthorName) > 0 {
		r.author = doc.AuthorName[0]
	}
	return r, nil
}
