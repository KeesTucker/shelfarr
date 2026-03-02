// Package metadata resolves audiobook metadata from OpenLibrary and Audnexus
// in parallel and merges the results. It is called on download completion to
// populate the folder-naming and notification fields for a request.
package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultOpenLibraryBase = "https://openlibrary.org"
)

// Book holds the merged audiobook metadata from all sources. It is
// JSON-serialised and stored in requests.metadata_json, then used by the
// file mover (step 7) for folder naming and by Discord for notifications.
type Book struct {
	Title    string `json:"title"`
	Author   string `json:"author"`
	Year     int    `json:"year,omitempty"`
	Narrator string `json:"narrator,omitempty"`
	Series   string `json:"series,omitempty"`
}

// Client fetches and merges audiobook metadata.
type Client struct {
	http            *http.Client
	openLibraryBase string
	audnexusBase    string
}

// New returns a Client with a 10-second timeout on all external calls.
// Pass a non-empty audnexusURL to enable Audnexus narrator/series lookups;
// an empty string disables Audnexus and only OpenLibrary is queried.
func New(audnexusURL string) *Client {
	return &Client{
		http:            &http.Client{Timeout: 10 * time.Second},
		openLibraryBase: defaultOpenLibraryBase,
		audnexusBase:    audnexusURL,
	}
}

// Resolve queries OpenLibrary and Audnexus concurrently and returns merged
// metadata. Source failures are logged as warnings but do not return an error
// — partial results are always preferred over nothing. The caller-supplied
// title and author are used as fallback values when a source returns no data.
func (c *Client) Resolve(ctx context.Context, title, author string) *Book {
	var (
		ol *olResult
		an *anResult
		wg sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		ol, err = c.openLibrarySearch(ctx, title, author)
		if err != nil {
			slog.Warn("metadata: OpenLibrary lookup failed", "title", title, "err", err)
		}
	}()

	if c.audnexusBase != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			an, err = c.audnexusSearch(ctx, title, author)
			if err != nil {
				slog.Warn("metadata: Audnexus lookup failed", "title", title, "err", err)
			}
		}()
	}

	wg.Wait()
	return merge(title, author, ol, an)
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

// merge combines results with source-specific preferences:
//   - Title, Author, Year → OpenLibrary (canonical bibliographic data)
//   - Narrator, Series   → Audnexus   (audiobook-specific metadata)
func merge(title, author string, ol *olResult, an *anResult) *Book {
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
	if an != nil {
		b.Narrator = an.narrator
		b.Series = an.series
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
	req.Header.Set("User-Agent", "bookarr/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

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

// ── Audnexus ──────────────────────────────────────────────────────────────────

type anResult struct {
	narrator string
	series   string
}

// anBook is the subset of an Audnexus book object we care about.
type anBook struct {
	Title     string `json:"title"`
	Narrators []struct {
		Name string `json:"name"`
	} `json:"narrators"`
	SeriesPrimary *struct {
		Name string `json:"name"`
	} `json:"seriesPrimary"`
}

// audnexusSearch queries https://api.audnexus.com/books by title and author.
// Audnexus is primarily ASIN-based; title search is best-effort and returns
// nil (not an error) when no match is found or the endpoint is unsupported.
func (c *Client) audnexusSearch(ctx context.Context, title, author string) (*anResult, error) {
	q := url.Values{
		"region": {"us"},
		"title":  {title},
	}
	if author != "" {
		q.Set("author", author)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.audnexusBase+"/books?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "bookarr/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // audiobook not indexed in Audnexus
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var book anBook
	if err := json.NewDecoder(resp.Body).Decode(&book); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	// If title and narrators are both absent the book object is empty/invalid.
	if book.Title == "" && len(book.Narrators) == 0 {
		return nil, nil
	}

	r := &anResult{}
	if len(book.Narrators) > 0 {
		names := make([]string, len(book.Narrators))
		for i, n := range book.Narrators {
			names[i] = n.Name
		}
		r.narrator = strings.Join(names, ", ")
	}
	if book.SeriesPrimary != nil {
		r.series = book.SeriesPrimary.Name
	}
	return r, nil
}
