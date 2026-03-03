package metadata

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// ── fake server helpers ───────────────────────────────────────────────────────

// fakeOL starts an httptest server that mimics the OpenLibrary search endpoint.
// docs is the slice of documents returned inside the "docs" key. Pass nil to
// simulate a no-results response.
func fakeOL(t *testing.T, docs []olDoc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search.json" {
			http.NotFound(w, r)
			return
		}
		resp := olResponse{Docs: docs}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv
}

type olDoc = struct {
	Title            string   `json:"title"`
	AuthorName       []string `json:"author_name"`
	FirstPublishYear int      `json:"first_publish_year"`
}

// newTestClient returns a Client pointed at the provided fake OL server.
func newTestClient(olURL string) *Client {
	return &Client{
		http:            &http.Client{},
		openLibraryBase: olURL,
	}
}

// ── merge (pure function) ─────────────────────────────────────────────────────

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		author   string
		ol       *olResult
		wantBook Book
	}{
		{
			name:     "nil result — fallback to caller values",
			title:    "Mistborn",
			author:   "Brandon Sanderson",
			wantBook: Book{Title: "Mistborn", Author: "Brandon Sanderson"},
		},
		{
			name:     "OL result",
			title:    "fallback",
			author:   "fallback",
			ol:       &olResult{title: "Mistborn", author: "Brandon Sanderson", year: 2006},
			wantBook: Book{Title: "Mistborn", Author: "Brandon Sanderson", Year: 2006},
		},
		{
			name:     "OL empty title falls back to caller title",
			title:    "caller title",
			author:   "caller author",
			ol:       &olResult{title: "", author: "", year: 2010},
			wantBook: Book{Title: "caller title", Author: "caller author", Year: 2010},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := merge(tc.title, tc.author, tc.ol)
			if *got != tc.wantBook {
				t.Errorf("got %+v; want %+v", *got, tc.wantBook)
			}
		})
	}
}

// ── Resolve (integration against fake HTTP server) ────────────────────────────

func TestResolve(t *testing.T) {
	olSrv := fakeOL(t, []olDoc{{
		Title:            "The Final Empire",
		AuthorName:       []string{"Brandon Sanderson"},
		FirstPublishYear: 2006,
	}})

	c := newTestClient(olSrv.URL)
	book := c.Resolve(context.Background(), "Mistborn", "Brandon Sanderson")

	if book.Title != "The Final Empire" {
		t.Errorf("Title=%q; want %q", book.Title, "The Final Empire")
	}
	if book.Author != "Brandon Sanderson" {
		t.Errorf("Author=%q; want %q", book.Author, "Brandon Sanderson")
	}
	if book.Year != 2006 {
		t.Errorf("Year=%d; want 2006", book.Year)
	}
}

// TestResolveOLFails: if OpenLibrary is down the caller-supplied values are
// returned and the call itself does not error.
func TestResolveOLFails(t *testing.T) {
	olSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(olSrv.Close)

	c := newTestClient(olSrv.URL)
	book := c.Resolve(context.Background(), "Mistborn", "Brandon Sanderson")

	if book.Title != "Mistborn" {
		t.Errorf("Title=%q; want caller fallback %q", book.Title, "Mistborn")
	}
	if book.Year != 0 {
		t.Errorf("Year=%d; want 0 (OL failed)", book.Year)
	}
}

// TestResolveOLNoResults: OL returns an empty docs array — no error, fallbacks used.
func TestResolveOLNoResults(t *testing.T) {
	olSrv := fakeOL(t, nil)

	c := newTestClient(olSrv.URL)
	book := c.Resolve(context.Background(), "Obscure Book", "Unknown Author")

	if book.Title != "Obscure Book" || book.Author != "Unknown Author" {
		t.Errorf("expected caller fallbacks, got %+v", *book)
	}
}

// TestResolvePassesQueryParams: the OL request must include title and author
// query parameters.
func TestResolvePassesQueryParams(t *testing.T) {
	var gotQuery url.Values
	olSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(olResponse{})
	}))
	t.Cleanup(olSrv.Close)

	c := newTestClient(olSrv.URL)
	c.Resolve(context.Background(), "Dune", "Frank Herbert")

	if gotQuery.Get("title") != "Dune" {
		t.Errorf("OL title param=%q; want %q", gotQuery.Get("title"), "Dune")
	}
	if gotQuery.Get("author") != "Frank Herbert" {
		t.Errorf("OL author param=%q; want %q", gotQuery.Get("author"), "Frank Herbert")
	}
}

// ── Book.JSON ─────────────────────────────────────────────────────────────────

func TestBookJSON(t *testing.T) {
	b := &Book{
		Title:  "Mistborn",
		Author: "Brandon Sanderson",
		Year:   2006,
	}
	raw, err := b.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var got Book
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got != *b {
		t.Errorf("round-trip: got %+v; want %+v", got, *b)
	}
}

func TestBookJSONOmitsZeroFields(t *testing.T) {
	b := &Book{Title: "Dune", Author: "Frank Herbert"} // Year zero
	raw, err := b.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := m["year"]; present {
		t.Error("JSON should omit zero-value field \"year\"")
	}
}
