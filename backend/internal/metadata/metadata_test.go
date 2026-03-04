package metadata

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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

// ── FromJSON ──────────────────────────────────────────────────────────────────

func TestFromJSON_RoundTrip(t *testing.T) {
	original := &Book{Title: "Mistborn", Author: "Brandon Sanderson", Year: 2006}
	raw, _ := original.JSON()

	got, err := FromJSON(raw)
	if err != nil {
		t.Fatalf("FromJSON error: %v", err)
	}
	if *got != *original {
		t.Errorf("got %+v; want %+v", *got, *original)
	}
}

func TestFromJSON_NoYear(t *testing.T) {
	got, err := FromJSON(`{"title":"Dune","author":"Frank Herbert"}`)
	if err != nil {
		t.Fatalf("FromJSON error: %v", err)
	}
	if got.Year != 0 {
		t.Errorf("Year=%d; want 0", got.Year)
	}
}

func TestFromJSON_InvalidJSON(t *testing.T) {
	_, err := FromJSON("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

// ── writeOPF / EnsureOPF ─────────────────────────────────────────────────────

// opfPackage is a minimal struct for parsing the generated OPF file.
// The dc: elements live in the Dublin Core namespace.
type opfPackage struct {
	XMLName          xml.Name    `xml:"package"`
	UniqueIdentifier string      `xml:"unique-identifier,attr"`
	Version          string      `xml:"version,attr"`
	Metadata         opfMetadata `xml:"metadata"`
}

type opfMetadata struct {
	Identifier   string `xml:"http://purl.org/dc/elements/1.1/ identifier"`
	IdentifierID string // populated from raw attr scan — see readOPF helper
	Title        string `xml:"http://purl.org/dc/elements/1.1/ title"`
	Creator      string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Date         string `xml:"http://purl.org/dc/elements/1.1/ date"`
	Language     string `xml:"http://purl.org/dc/elements/1.1/ language"`
}

// readOPF parses the OPF file at path and returns the package element.
func readOPF(t *testing.T, path string) opfPackage {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		t.Fatalf("read OPF: %v", err)
	}
	var pkg opfPackage
	if err := xml.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse OPF as XML: %v\ncontent:\n%s", err, data)
	}
	return pkg
}

func TestWriteOPF_ValidXML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "book.opf")
	book := &Book{Title: "Mistborn", Author: "Brandon Sanderson", Year: 2006}
	if err := writeOPF(path, book); err != nil {
		t.Fatalf("writeOPF: %v", err)
	}
	// xml.Unmarshal errors on malformed XML — readOPF calls t.Fatal on failure.
	readOPF(t, path)
}

func TestWriteOPF_Fields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "book.opf")
	book := &Book{Title: "The Final Empire", Author: "Brandon Sanderson", Year: 2006}
	if err := writeOPF(path, book); err != nil {
		t.Fatalf("writeOPF: %v", err)
	}

	pkg := readOPF(t, path)

	if pkg.Metadata.Title != book.Title {
		t.Errorf("dc:title=%q; want %q", pkg.Metadata.Title, book.Title)
	}
	if pkg.Metadata.Creator != book.Author {
		t.Errorf("dc:creator=%q; want %q", pkg.Metadata.Creator, book.Author)
	}
	if pkg.Metadata.Date != "2006-01-01" {
		t.Errorf("dc:date=%q; want %q", pkg.Metadata.Date, "2006-01-01")
	}
	if pkg.Metadata.Language != "en" {
		t.Errorf("dc:language=%q; want %q", pkg.Metadata.Language, "en")
	}
}

func TestWriteOPF_NoYear_DateElementAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "book.opf")
	if err := writeOPF(path, &Book{Title: "Dune", Author: "Frank Herbert"}); err != nil {
		t.Fatalf("writeOPF: %v", err)
	}

	pkg := readOPF(t, path)
	if pkg.Metadata.Date != "" {
		t.Errorf("dc:date=%q; want empty (year==0)", pkg.Metadata.Date)
	}
}

func TestWriteOPF_UniqueIdentifierReferencesIdentifierElement(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "book.opf")
	if err := writeOPF(path, &Book{Title: "Dune", Author: "Frank Herbert", Year: 1965}); err != nil {
		t.Fatalf("writeOPF: %v", err)
	}

	pkg := readOPF(t, path)

	// The unique-identifier attribute must point to "uid".
	if pkg.UniqueIdentifier != "uid" {
		t.Errorf("unique-identifier=%q; want %q", pkg.UniqueIdentifier, "uid")
	}
	// And the dc:identifier element must exist (non-empty) to satisfy that reference.
	if pkg.Metadata.Identifier == "" {
		t.Error("dc:identifier element is missing or empty; unique-identifier would be a dangling reference")
	}
	// The id attribute on dc:identifier must be "uid".
	data, _ := os.ReadFile(path) //nolint:gosec
	if !strings.Contains(string(data), `id="uid"`) {
		t.Error(`dc:identifier element must have id="uid" to satisfy unique-identifier="uid"`)
	}
}

func TestWriteOPF_SpecialCharsEscaped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "book.opf")
	book := &Book{Title: "A & B: <Story>", Author: `Author "Quote"`, Year: 2000}
	if err := writeOPF(path, book); err != nil {
		t.Fatalf("writeOPF: %v", err)
	}

	// XML parser must succeed (would fail on unescaped & < >).
	pkg := readOPF(t, path)

	// Values should be round-tripped correctly by the XML parser.
	if pkg.Metadata.Title != book.Title {
		t.Errorf("dc:title=%q; want %q", pkg.Metadata.Title, book.Title)
	}
	if pkg.Metadata.Creator != book.Author {
		t.Errorf("dc:creator=%q; want %q", pkg.Metadata.Creator, book.Author)
	}
}

func TestEnsureOPF_CreatesWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureOPF(dir, &Book{Title: "Dune", Author: "Frank Herbert", Year: 1965}); err != nil {
		t.Fatalf("EnsureOPF: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(dir, "*.opf"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 OPF file, got %d", len(matches))
	}
	pkg := readOPF(t, matches[0])
	if pkg.Metadata.Title != "Dune" {
		t.Errorf("dc:title=%q; want %q", pkg.Metadata.Title, "Dune")
	}
}

func TestEnsureOPF_SkipsWhenAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing.opf")
	if err := os.WriteFile(existing, []byte("<?xml version='1.0'?><package/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureOPF(dir, &Book{Title: "Dune", Author: "Frank Herbert"}); err != nil {
		t.Fatalf("EnsureOPF: %v", err)
	}

	// book.opf must NOT have been created — the existing file should be the only one.
	matches, _ := filepath.Glob(filepath.Join(dir, "*.opf"))
	if len(matches) != 1 {
		t.Errorf("expected 1 OPF file (the pre-existing one), got %d: %v", len(matches), matches)
	}
	if matches[0] != existing {
		t.Errorf("expected pre-existing %q to be untouched, got %q", existing, matches[0])
	}
}
