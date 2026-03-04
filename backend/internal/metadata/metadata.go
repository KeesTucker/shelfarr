// Package metadata resolves audiobook metadata from OpenLibrary and stores it
// for folder-naming and notification fields on a request.
package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
	results, err := c.openLibraryDocs(ctx, title, author, 1)
	if err != nil {
		slog.Warn("metadata: OpenLibrary lookup failed", "title", title, "err", err)
		return &Book{Title: title, Author: author}
	}
	if len(results) == 0 {
		return &Book{Title: title, Author: author}
	}
	return merge(title, author, &results[0])
}

// Search queries OpenLibrary and returns up to 5 candidate Books for the user
// to choose from. Returns nil (not an error) when no results are found.
func (c *Client) Search(ctx context.Context, title, author string) ([]Book, error) {
	docs, err := c.openLibraryDocs(ctx, title, author, 5)
	if err != nil {
		return nil, err
	}
	books := make([]Book, 0, len(docs))
	for i := range docs {
		books = append(books, Book{
			Title:  docs[i].title,
			Author: docs[i].author,
			Year:   docs[i].year,
		})
	}
	return books, nil
}

// JSON serialises the Book to a JSON string for storage in metadata_json.
func (b *Book) JSON() (string, error) {
	raw, err := json.Marshal(b)
	if err != nil {
		return "", fmt.Errorf("marshal book: %w", err)
	}
	return string(raw), nil
}

// FromJSON deserialises a Book from a JSON string previously produced by JSON().
func FromJSON(s string) (*Book, error) {
	var b Book
	if err := json.Unmarshal([]byte(s), &b); err != nil {
		return nil, fmt.Errorf("unmarshal book: %w", err)
	}
	return &b, nil
}

// EnsureOPF writes a book.opf sidecar into dir if no *.opf file already
// exists there. Non-fatal: callers should log and ignore errors.
func EnsureOPF(dir string, book *Book) error {
	matches, err := filepath.Glob(filepath.Join(dir, "*.opf"))
	if err != nil {
		return fmt.Errorf("glob opf: %w", err)
	}
	if len(matches) > 0 {
		return nil // already present
	}
	return writeOPF(filepath.Join(dir, "book.opf"), book)
}

func writeOPF(path string, book *Book) error {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	buf.WriteString(`<package xmlns="http://www.idpf.org/2007/opf" xmlns:opf="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0" unique-identifier="uid">` + "\n")
	buf.WriteString("  <metadata>\n")
	// unique-identifier must match an id= on a dc:identifier element.
	fmt.Fprintf(&buf, "    <dc:identifier id=\"uid\">%s</dc:identifier>\n",
		html.EscapeString(book.Author+" - "+book.Title))
	fmt.Fprintf(&buf, "    <dc:title>%s</dc:title>\n", html.EscapeString(book.Title))
	fmt.Fprintf(&buf, "    <dc:creator opf:role=\"aut\">%s</dc:creator>\n", html.EscapeString(book.Author))
	if book.Year > 0 {
		fmt.Fprintf(&buf, "    <dc:date>%d-01-01</dc:date>\n", book.Year)
	}
	buf.WriteString("    <dc:language>en</dc:language>\n")
	buf.WriteString("  </metadata>\n")
	buf.WriteString("</package>\n")
	return os.WriteFile(path, buf.Bytes(), 0o644) //nolint:gosec
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

// openLibraryDocs queries OpenLibrary with the given limit and returns raw results.
func (c *Client) openLibraryDocs(ctx context.Context, title, author string, limit int) ([]olResult, error) {
	q := url.Values{
		"title":  {title},
		"author": {author},
		"fields": {"title,author_name,first_publish_year"},
		"limit":  {fmt.Sprintf("%d", limit)},
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

	results := make([]olResult, 0, len(body.Docs))
	for _, doc := range body.Docs {
		r := olResult{title: doc.Title, year: doc.FirstPublishYear}
		if len(doc.AuthorName) > 0 {
			r.author = doc.AuthorName[0]
		}
		results = append(results, r)
	}
	return results, nil
}
