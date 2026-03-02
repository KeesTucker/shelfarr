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

// fakeAN starts an httptest server that mimics the Audnexus /books endpoint.
// book is returned as the JSON response body. Pass nil to respond with 404.
func fakeAN(t *testing.T, book *anBook) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/books" {
			http.NotFound(w, r)
			return
		}
		if book == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(book)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTestClient returns a Client pointed at the provided fake servers.
// Pass an empty string for anURL to disable the Audnexus lookup.
func newTestClient(olURL, anURL string) *Client {
	return &Client{
		http:            &http.Client{},
		openLibraryBase: olURL,
		audnexusBase:    anURL,
	}
}

// ── merge (pure function) ─────────────────────────────────────────────────────

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		author   string
		ol       *olResult
		an       *anResult
		wantBook Book
	}{
		{
			name:     "both sources nil — fallback to caller values",
			title:    "Mistborn",
			author:   "Brandon Sanderson",
			wantBook: Book{Title: "Mistborn", Author: "Brandon Sanderson"},
		},
		{
			name:     "OL only",
			title:    "fallback",
			author:   "fallback",
			ol:       &olResult{title: "Mistborn", author: "Brandon Sanderson", year: 2006},
			wantBook: Book{Title: "Mistborn", Author: "Brandon Sanderson", Year: 2006},
		},
		{
			name:     "AN only — narrator and series filled, title/author from caller",
			title:    "Mistborn",
			author:   "Brandon Sanderson",
			an:       &anResult{narrator: "Michael Kramer", series: "Mistborn"},
			wantBook: Book{Title: "Mistborn", Author: "Brandon Sanderson", Narrator: "Michael Kramer", Series: "Mistborn"},
		},
		{
			name:   "OL wins title/author/year, AN wins narrator/series",
			title:  "fallback",
			author: "fallback",
			ol:     &olResult{title: "The Final Empire", author: "Brandon Sanderson", year: 2006},
			an:     &anResult{narrator: "Michael Kramer", series: "Mistborn"},
			wantBook: Book{
				Title:    "The Final Empire",
				Author:   "Brandon Sanderson",
				Year:     2006,
				Narrator: "Michael Kramer",
				Series:   "Mistborn",
			},
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
			got := merge(tc.title, tc.author, tc.ol, tc.an)
			if *got != tc.wantBook {
				t.Errorf("got %+v; want %+v", *got, tc.wantBook)
			}
		})
	}
}

// ── Resolve (integration against fake HTTP servers) ───────────────────────────

func TestResolveBothSources(t *testing.T) {
	olSrv := fakeOL(t, []olDoc{{
		Title:            "The Final Empire",
		AuthorName:       []string{"Brandon Sanderson"},
		FirstPublishYear: 2006,
	}})
	anSrv := fakeAN(t, &anBook{
		Title: "The Final Empire",
		Narrators: []struct {
			Name string `json:"name"`
		}{{Name: "Michael Kramer"}},
		SeriesPrimary: &struct {
			Name string `json:"name"`
		}{Name: "Mistborn"},
	})

	c := newTestClient(olSrv.URL, anSrv.URL)
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
	if book.Narrator != "Michael Kramer" {
		t.Errorf("Narrator=%q; want %q", book.Narrator, "Michael Kramer")
	}
	if book.Series != "Mistborn" {
		t.Errorf("Series=%q; want %q", book.Series, "Mistborn")
	}
}

// TestResolveOLFailsGracefully: if OpenLibrary is down, Audnexus data is still
// used and the caller-supplied title/author serve as fallbacks.
func TestResolveOLFailsGracefully(t *testing.T) {
	olSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(olSrv.Close)

	anSrv := fakeAN(t, &anBook{
		Title: "Mistborn",
		Narrators: []struct {
			Name string `json:"name"`
		}{{Name: "Michael Kramer"}},
	})

	c := newTestClient(olSrv.URL, anSrv.URL)
	book := c.Resolve(context.Background(), "Mistborn", "Brandon Sanderson")

	// Title/Author come from the caller fallback; Year is zero because OL failed.
	if book.Title != "Mistborn" {
		t.Errorf("Title=%q; want %q (caller fallback)", book.Title, "Mistborn")
	}
	if book.Year != 0 {
		t.Errorf("Year=%d; want 0 (OL failed)", book.Year)
	}
	if book.Narrator != "Michael Kramer" {
		t.Errorf("Narrator=%q; want %q", book.Narrator, "Michael Kramer")
	}
}

// TestResolveANFailsGracefully: if Audnexus is down, OL data is still used and
// Narrator/Series are simply empty.
func TestResolveANFailsGracefully(t *testing.T) {
	olSrv := fakeOL(t, []olDoc{{
		Title:            "Mistborn",
		AuthorName:       []string{"Brandon Sanderson"},
		FirstPublishYear: 2006,
	}})
	anSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	t.Cleanup(anSrv.Close)

	c := newTestClient(olSrv.URL, anSrv.URL)
	book := c.Resolve(context.Background(), "Mistborn", "Brandon Sanderson")

	if book.Year != 2006 {
		t.Errorf("Year=%d; want 2006 (OL still worked)", book.Year)
	}
	if book.Narrator != "" {
		t.Errorf("Narrator=%q; want empty (AN failed)", book.Narrator)
	}
}

// TestResolveBothFail: when both sources fail the caller-supplied values are
// returned and the call itself does not error.
func TestResolveBothFail(t *testing.T) {
	fail := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	olSrv := httptest.NewServer(fail)
	t.Cleanup(olSrv.Close)
	anSrv := httptest.NewServer(fail)
	t.Cleanup(anSrv.Close)

	c := newTestClient(olSrv.URL, anSrv.URL)
	book := c.Resolve(context.Background(), "Mistborn", "Brandon Sanderson")

	if book.Title != "Mistborn" || book.Author != "Brandon Sanderson" {
		t.Errorf("got %+v; want caller-supplied title/author as fallback", *book)
	}
	if book.Year != 0 || book.Narrator != "" || book.Series != "" {
		t.Errorf("optional fields should be zero when both sources fail, got %+v", *book)
	}
}

// TestResolveOLNoResults: OL returns an empty docs array — no error, just nil result.
func TestResolveOLNoResults(t *testing.T) {
	olSrv := fakeOL(t, nil) // empty docs
	anSrv := fakeAN(t, nil) // 404

	c := newTestClient(olSrv.URL, anSrv.URL)
	book := c.Resolve(context.Background(), "Obscure Book", "Unknown Author")

	if book.Title != "Obscure Book" || book.Author != "Unknown Author" {
		t.Errorf("expected caller fallbacks, got %+v", *book)
	}
}

// TestResolveMultipleNarrators: multiple narrators are joined with ", ".
func TestResolveMultipleNarrators(t *testing.T) {
	olSrv := fakeOL(t, nil)
	anSrv := fakeAN(t, &anBook{
		Title: "Good Omens",
		Narrators: []struct {
			Name string `json:"name"`
		}{{Name: "Martin Jarvis"}, {Name: "Nigel Planer"}},
	})

	c := newTestClient(olSrv.URL, anSrv.URL)
	book := c.Resolve(context.Background(), "Good Omens", "Terry Pratchett")

	want := "Martin Jarvis, Nigel Planer"
	if book.Narrator != want {
		t.Errorf("Narrator=%q; want %q", book.Narrator, want)
	}
}

// TestResolvePassesQueryParams: the OL request must include title and author
// query parameters (spot-checked via the fake server).
func TestResolvePassesQueryParams(t *testing.T) {
	var gotQuery url.Values
	olSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(olResponse{})
	}))
	t.Cleanup(olSrv.Close)

	anSrv := fakeAN(t, nil)
	c := newTestClient(olSrv.URL, anSrv.URL)
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
		Title:    "Mistborn",
		Author:   "Brandon Sanderson",
		Year:     2006,
		Narrator: "Michael Kramer",
		Series:   "Mistborn",
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
	b := &Book{Title: "Dune", Author: "Frank Herbert"} // Year/Narrator/Series zero
	raw, err := b.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"year", "narrator", "series"} {
		if _, present := m[key]; present {
			t.Errorf("JSON should omit zero-value field %q", key)
		}
	}
}
