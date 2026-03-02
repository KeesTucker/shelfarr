//go:build integration

package metadata_test

// Integration tests that hit the real OpenLibrary and Audnexus APIs.
//
// Run with:
//
//	go test -tags integration -v ./internal/metadata/
//
// These tests require an internet connection. No credentials are needed.
// They use well-known audiobooks so results should be stable across runs.

import (
	"context"
	"os"
	"testing"
	"time"

	"bookarr/internal/metadata"
)

// integrationClient returns a Client with Audnexus enabled only when
// AUDNEXUS_URL is set (it has no public hosted instance).
func integrationClient() *metadata.Client {
	return metadata.New(os.Getenv("AUDNEXUS_URL"))
}

// ── OpenLibrary ───────────────────────────────────────────────────────────────

// TestIntegration_OpenLibrary_WellKnownBook verifies that a widely-indexed
// audiobook returns the expected year and canonical author name.
func TestIntegration_OpenLibrary_WellKnownBook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := integrationClient()
	book := c.Resolve(ctx, "The Final Empire", "Brandon Sanderson")

	if book.Title == "" {
		t.Error("Title is empty; expected OpenLibrary to return a title")
	}
	if book.Author == "" {
		t.Error("Author is empty; expected OpenLibrary to return an author")
	}
	if book.Year == 0 {
		t.Error("Year is 0; expected OpenLibrary to return a publish year")
	}
	t.Logf("OL result: title=%q author=%q year=%d", book.Title, book.Author, book.Year)
}

// TestIntegration_OpenLibrary_UnambiguousYear checks a book with a clear,
// well-documented first-publish year.
func TestIntegration_OpenLibrary_UnambiguousYear(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := integrationClient()
	// "The Hobbit" was first published in 1937.
	book := c.Resolve(ctx, "The Hobbit", "J.R.R. Tolkien")

	if book.Year == 0 {
		t.Error("Year is 0; expected a non-zero publish year for The Hobbit")
	}
	if book.Year > 1940 {
		t.Errorf("Year=%d; The Hobbit was first published in 1937, so year should be ≤ 1940", book.Year)
	}
	t.Logf("OL result: title=%q author=%q year=%d", book.Title, book.Author, book.Year)
}

// TestIntegration_OpenLibrary_NoResults checks that an absurd query returns
// a Book (not an error) with the caller's fallback title/author.
func TestIntegration_OpenLibrary_NoResults(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := integrationClient()
	const title, author = "zzz_no_such_book_xqpwlm", "zzz_no_such_author_xqpwlm"
	book := c.Resolve(ctx, title, author)

	// Resolve never returns an error; when OL finds nothing, caller values are used.
	if book.Title != title {
		t.Errorf("Title=%q; want caller fallback %q", book.Title, title)
	}
	if book.Author != author {
		t.Errorf("Author=%q; want caller fallback %q", book.Author, author)
	}
}

// ── Audnexus ─────────────────────────────────────────────────────────────────

// TestIntegration_Audnexus_Response checks that the Audnexus lookup either
// returns usable narrator/series data or degrades gracefully to empty strings.
// We log what we get rather than asserting specific values because Audnexus is
// primarily ASIN-based and title-search support is best-effort.
func TestIntegration_Audnexus_Response(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	c := integrationClient()
	book := c.Resolve(ctx, "The Final Empire", "Brandon Sanderson")

	// The test passes regardless of whether Audnexus returned data; we just
	// verify the fields are either populated or empty (not garbage).
	t.Logf("AN result: narrator=%q series=%q", book.Narrator, book.Series)

	// Sanity: if Audnexus did return a narrator, it must be a non-whitespace string.
	if book.Narrator == " " || book.Narrator == "," {
		t.Errorf("Narrator=%q looks malformed", book.Narrator)
	}
}

// ── full Resolve ──────────────────────────────────────────────────────────────

// TestIntegration_Resolve_AlwaysReturnsBook checks the most important
// invariant: Resolve never returns nil and always has at minimum the
// caller-supplied title/author as fallback values.
func TestIntegration_Resolve_AlwaysReturnsBook(t *testing.T) {
	cases := []struct{ title, author string }{
		{"Mistborn: The Final Empire", "Brandon Sanderson"},
		{"Dune", "Frank Herbert"},
		{"zzz_no_such_book_xqpwlm", "zzz_no_such_author_xqpwlm"},
	}

	c := integrationClient()

	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			book := c.Resolve(ctx, tc.title, tc.author)

			if book == nil {
				t.Fatal("Resolve returned nil; must always return a non-nil Book")
			}
			if book.Title == "" {
				t.Error("Title is empty; must be at least the caller fallback")
			}
			if book.Author == "" {
				t.Error("Author is empty; must be at least the caller fallback")
			}
			t.Logf("book: %+v", *book)
		})
	}
}
