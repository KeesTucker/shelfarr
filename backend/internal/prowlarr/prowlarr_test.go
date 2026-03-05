package prowlarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ── parseTitle ────────────────────────────────────────────────────────────────

func TestParseTitle(t *testing.T) {
	tests := []struct {
		raw      string
		title    string
		author   string
		narrator string
	}{
		{
			raw:      "Brandon Sanderson - The Final Empire - Michael Kramer",
			title:    "The Final Empire",
			author:   "Brandon Sanderson",
			narrator: "Michael Kramer",
		},
		{
			raw:      "Brandon Sanderson - The Final Empire",
			title:    "The Final Empire",
			author:   "Brandon Sanderson",
			narrator: "",
		},
		{
			raw:      "The Name of the Wind by Patrick Rothfuss",
			title:    "The Name of the Wind",
			author:   "Patrick Rothfuss",
			narrator: "",
		},
		{
			raw:      "Brandon Sanderson - Mistborn 01 - The Final Empire - Michael Kramer [Audiobook]",
			title:    "Mistborn 01 - The Final Empire",
			author:   "Brandon Sanderson",
			narrator: "Michael Kramer",
		},
		{
			// "Author - Title" form after narrator is extracted.
			// The parser treats the first dash-segment as author.
			raw:      "Dune - Frank Herbert - Narrated by Scott Brick [MP3]",
			title:    "Frank Herbert",
			author:   "Dune",
			narrator: "Scott Brick",
		},
		{
			raw:      "The Hitchhiker's Guide to the Galaxy (2005) [Unabridged]",
			title:    "The Hitchhiker's Guide to the Galaxy",
			author:   "",
			narrator: "",
		},
		{
			// No separators at all — falls back to stripped raw.
			raw:      "SomeAudiobook.m4b",
			title:    "SomeAudiobook.m4b",
			author:   "",
			narrator: "",
		},
		{
			// Inline language/format tag on the author segment.
			raw:    "J K Rowling [ENG / MP3] - Harry Potter and the Sorcerer's Stone",
			title:  "Harry Potter and the Sorcerer's Stone",
			author: "J K Rowling",
		},
		{
			// Inline tag at the end of the full title.
			raw:    "J K Rowling - Harry Potter and the Sorcerer's Stone [ENG / MP3]",
			title:  "Harry Potter and the Sorcerer's Stone",
			author: "J K Rowling",
		},
		{
			// Tag with only a language code.
			raw:    "Terry Pratchett [ENG] - Guards! Guards!",
			title:  "Guards! Guards!",
			author: "Terry Pratchett",
		},
	}

	for _, tc := range tests {
		t.Run(tc.raw, func(t *testing.T) {
			title, author, narrator := parseTitle(tc.raw)
			if title != tc.title {
				t.Errorf("title: got %q, want %q", title, tc.title)
			}
			if author != tc.author {
				t.Errorf("author: got %q, want %q", author, tc.author)
			}
			if narrator != tc.narrator {
				t.Errorf("narrator: got %q, want %q", narrator, tc.narrator)
			}
		})
	}
}

func TestParseTitleNeverEmpty(t *testing.T) {
	// title must always be non-empty for non-empty raw inputs.
	cases := []string{"- -", "[Audiobook]"}
	for _, raw := range cases {
		title, _, _ := parseTitle(raw)
		if title == "" {
			t.Errorf("parseTitle(%q): got empty title", raw)
		}
	}
}

// ── looksLikeName ─────────────────────────────────────────────────────────────

func TestLooksLikeName(t *testing.T) {
	yes := []string{"Michael Kramer", "Kate Reading", "Simon Vance", "R.C. Bray"}
	no := []string{"2006", "[MP3]", "The Final Empire", "A", "X Y", "AudioBook Unabridged", "Unabridged"}

	for _, s := range yes {
		if !looksLikeName(s) {
			t.Errorf("looksLikeName(%q): expected true", s)
		}
	}
	for _, s := range no {
		if looksLikeName(s) {
			t.Errorf("looksLikeName(%q): expected false", s)
		}
	}
}

// ── rank ──────────────────────────────────────────────────────────────────────

func TestRankSeeders(t *testing.T) {
	releases := []Release{
		{GUID: "a", Title: "Book A - Author A", Seeders: 5},
		{GUID: "b", Title: "Book B - Author B", Seeders: 50},
		{GUID: "c", Title: "Book C - Author C", Seeders: 10},
	}
	results := rank(releases, "audiobook")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0].ID != "b" {
		t.Errorf("expected highest-seeder first, got ID %q", results[0].ID)
	}
	if results[2].ID != "a" {
		t.Errorf("expected lowest-seeder last, got ID %q", results[2].ID)
	}
}

func TestRankAbridgedPenalty(t *testing.T) {
	releases := []Release{
		{GUID: "abridged", Title: "Great Book - Author (Abridged)", Seeders: 100},
		{GUID: "full", Title: "Great Book - Author", Seeders: 10},
		{GUID: "unabridged", Title: "Great Book - Author [Unabridged]", Seeders: 5},
	}
	results := rank(releases, "audiobook")

	// "full" (10 seeders, no penalty) should beat "abridged" (100 seeders - 1000 penalty = -900).
	// "unabridged" should not be penalised (it does not match "abridged" without "un").
	if results[0].ID == "abridged" {
		t.Error("abridged result should not rank first")
	}

	// Find abridged position — must be last.
	var abridgedPos int
	for i, r := range results {
		if r.ID == "abridged" {
			abridgedPos = i
		}
	}
	if abridgedPos != len(results)-1 {
		t.Errorf("abridged result should be last, but is at position %d", abridgedPos)
	}
}

func TestRankUnabridgedNotPenalized(t *testing.T) {
	releases := []Release{
		{GUID: "u", Title: "Book [Unabridged]", Seeders: 20},
		{GUID: "a", Title: "Book (Abridged)", Seeders: 10},
	}
	results := rank(releases, "audiobook")
	if results[0].ID != "u" {
		t.Errorf("unabridged with more seeders should rank first, got %q", results[0].ID)
	}
}

// ── Search handler ────────────────────────────────────────────────────────────

func TestSearchHandlerTypeParam(t *testing.T) {
	cases := []struct {
		typeParam string
		wantCat   string
	}{
		{"audiobook", "3030"},
		{"ebook", "7020"},
		{"", "3030"}, // defaults to audiobook
	}
	for _, tc := range cases {
		var gotCat string
		fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCat = r.URL.Query().Get("categories")
			_ = json.NewEncoder(w).Encode([]rawRelease{})
		}))
		h := NewHandler(New(fake.URL, "key"))
		url := "/api/search?q=test"
		if tc.typeParam != "" {
			url += "&type=" + tc.typeParam
		}
		req := httptest.NewRequest(http.MethodGet, url, nil)
		rr := httptest.NewRecorder()
		h.Search(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("type=%q: got %d, want 200", tc.typeParam, rr.Code)
		}
		if gotCat != tc.wantCat {
			t.Errorf("type=%q: categories=%q, want %q", tc.typeParam, gotCat, tc.wantCat)
		}
		fake.Close()
	}
}

func TestSearchHandlerMissingQ(t *testing.T) {
	h := NewHandler(New("http://fake", "key"))
	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSearchHandlerProwlarrDown(t *testing.T) {
	// Point at a server that immediately refuses connections.
	h := NewHandler(New("http://127.0.0.1:1", "key"))
	req := httptest.NewRequest(http.MethodGet, "/api/search?q=test", nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", rr.Code)
	}
}

func TestSearchHandlerOK(t *testing.T) {
	// Spin up a fake Prowlarr that returns two releases.
	fakeReleases := []rawRelease{
		{
			GUID:        "guid-1",
			Title:       "Brandon Sanderson - The Final Empire - Michael Kramer",
			Size:        500_000_000,
			Seeders:     42,
			Indexer:     "MyIndexer",
			PublishDate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
			DownloadURL: "magnet:?xt=urn:btih:abc",
		},
		{
			GUID:        "guid-2",
			Title:       "Brandon Sanderson - The Final Empire (Abridged)",
			Size:        100_000_000,
			Seeders:     100, // more seeders but abridged → lower rank
			Indexer:     "MyIndexer",
			PublishDate: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC),
			DownloadURL: "magnet:?xt=urn:btih:def",
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/search" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fakeReleases)
	}))
	defer srv.Close()

	client := New(srv.URL, "testkey")
	h := NewHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/api/search?q=Mistborn", nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var results []Result
	if err := json.NewDecoder(rr.Body).Decode(&results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Non-abridged (guid-1, 42 seeders) should outrank abridged (guid-2, 100 - 1000 = -900).
	if results[0].ID != "guid-1" {
		t.Errorf("expected guid-1 first, got %q", results[0].ID)
	}
	if results[0].Author != "Brandon Sanderson" {
		t.Errorf("expected author %q, got %q", "Brandon Sanderson", results[0].Author)
	}
	if results[0].Narrator != "Michael Kramer" {
		t.Errorf("expected narrator %q, got %q", "Michael Kramer", results[0].Narrator)
	}
}

// ── mediaType category routing ────────────────────────────────────────────────

func TestSearchSendsCorrectCategory(t *testing.T) {
	cases := []struct {
		mediaType string
		wantCat   string
	}{
		{"audiobook", "3030"},
		{"ebook", "7020"},
		{"", "3030"},      // empty defaults to audiobook
		{"other", "3030"}, // unknown defaults to audiobook
	}
	for _, tc := range cases {
		var gotCat string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotCat = r.URL.Query().Get("categories")
			_ = json.NewEncoder(w).Encode([]rawRelease{})
		}))
		client := New(srv.URL, "key")
		if _, err := client.Search(context.Background(), "test", tc.mediaType); err != nil {
			t.Fatalf("mediaType=%q Search error: %v", tc.mediaType, err)
		}
		if gotCat != tc.wantCat {
			t.Errorf("mediaType=%q: categories=%q, want %q", tc.mediaType, gotCat, tc.wantCat)
		}
		srv.Close()
	}
}

// ── Client cache ──────────────────────────────────────────────────────────────

func TestClientGetByGUID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		releases := []rawRelease{{GUID: "x", Title: "A - B", Seeders: 1, DownloadURL: "magnet:x"}}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer srv.Close()

	client := New(srv.URL, "key")
	if _, err := client.Search(context.Background(), "test", "audiobook"); err != nil {
		t.Fatal(err)
	}

	rel, ok := client.GetByGUID("x")
	if !ok {
		t.Fatal("expected GUID 'x' to be in cache after search")
	}
	if rel.DownloadURL != "magnet:x" {
		t.Errorf("unexpected DownloadURL: %q", rel.DownloadURL)
	}

	_, ok = client.GetByGUID("missing")
	if ok {
		t.Error("expected missing GUID to return false")
	}
}
