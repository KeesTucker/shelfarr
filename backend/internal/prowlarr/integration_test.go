//go:build integration

package prowlarr_test

// Integration tests that talk to a real Prowlarr instance.
//
// Run with:
//
//	PROWLARR_API_KEY=<key> go test -tags integration -v ./internal/prowlarr/
//
// Optional overrides:
//
//	PROWLARR_URL=http://... (default: http://10.10.10.2:9696)
//	TEST_SEARCH_QUERY=...   (default: The Hobbit)

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"shelfarr/internal/prowlarr"
)

// integrationClient creates a real Prowlarr client from env vars.
// The test is skipped if PROWLARR_API_KEY is not set.
func integrationClient(t *testing.T) *prowlarr.Client {
	t.Helper()
	apiKey := os.Getenv("PROWLARR_API_KEY")
	if apiKey == "" {
		t.Skip("PROWLARR_API_KEY not set; skipping integration test")
	}
	baseURL := os.Getenv("PROWLARR_URL")
	if baseURL == "" {
		baseURL = "http://10.10.10.2:9696"
	}
	return prowlarr.New(baseURL, apiKey)
}

func searchQuery() string {
	if q := os.Getenv("TEST_SEARCH_QUERY"); q != "" {
		return q
	}
	return "The Hobbit"
}

// ── client ────────────────────────────────────────────────────────────────────

func TestIntegration_ClientSearch(t *testing.T) {
	client := integrationClient(t)
	query := searchQuery()

	releases, err := client.Search(context.Background(), query, "audiobook")
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}

	t.Logf("query=%q → %d releases", query, len(releases))

	for i, r := range releases {
		if r.GUID == "" {
			t.Errorf("release[%d] has empty GUID (title=%q)", i, r.Title)
		}
		if r.Title == "" {
			t.Errorf("release[%d] has empty Title", i)
		}
		if r.DownloadURL == "" {
			t.Errorf("release[%d] %q has empty DownloadURL", i, r.GUID)
		}
		if r.Seeders < 0 {
			t.Errorf("release[%d] %q has negative Seeders: %d", i, r.GUID, r.Seeders)
		}
	}
}

func TestIntegration_ClientGUIDCache(t *testing.T) {
	client := integrationClient(t)
	query := searchQuery()

	releases, err := client.Search(context.Background(), query, "audiobook")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(releases) == 0 {
		t.Skip("no releases returned; cannot test GUID cache")
	}

	// Every non-empty GUID returned by Search must be retrievable via GetByGUID.
	for _, r := range releases {
		if r.GUID == "" {
			continue
		}
		cached, ok := client.GetByGUID(r.GUID)
		if !ok {
			t.Errorf("GUID %q not found in cache immediately after Search", r.GUID)
			continue
		}
		if cached.DownloadURL != r.DownloadURL {
			t.Errorf("GUID %q: cached DownloadURL %q != search DownloadURL %q",
				r.GUID, cached.DownloadURL, r.DownloadURL)
		}
	}

	// Unknown GUID must return false.
	if _, ok := client.GetByGUID("definitely-not-a-real-guid"); ok {
		t.Error("GetByGUID returned true for unknown GUID")
	}
}

// ── handler ───────────────────────────────────────────────────────────────────

func TestIntegration_HandlerSearch(t *testing.T) {
	client := integrationClient(t)
	h := prowlarr.NewHandler(client)
	query := searchQuery()

	req := httptest.NewRequest(http.MethodGet, "/api/search?q="+url.QueryEscape(query), nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", rr.Code, rr.Body)
	}

	var results []prowlarr.Result
	if err := json.NewDecoder(rr.Body).Decode(&results); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	t.Logf("query=%q → %d ranked results", query, len(results))

	for i, r := range results {
		if r.ID == "" {
			t.Errorf("results[%d] has empty ID", i)
		}
		if r.Title == "" {
			t.Errorf("results[%d] has empty Title", i)
		}
		t.Logf("[%d] seeders=%-4d abridged=%-5v title=%q author=%q narrator=%q",
			i, r.Seeders, isAbridged(r.Title), r.Title, r.Author, r.Narrator)
	}
}

func TestIntegration_HandlerRankingAbridgedLast(t *testing.T) {
	client := integrationClient(t)
	h := prowlarr.NewHandler(client)
	query := searchQuery()

	req := httptest.NewRequest(http.MethodGet, "/api/search?q="+url.QueryEscape(query), nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var results []prowlarr.Result
	if err := json.NewDecoder(rr.Body).Decode(&results); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Count abridged vs non-abridged.
	var abridgedCount, fullCount int
	for _, r := range results {
		if isAbridged(r.Title) {
			abridgedCount++
		} else {
			fullCount++
		}
	}

	if abridgedCount == 0 || fullCount == 0 {
		t.Skipf("need both abridged and non-abridged results to test ranking (abridged=%d full=%d)",
			abridgedCount, fullCount)
	}

	// Every non-abridged result must appear before every abridged result,
	// unless the abridged result has enough seeders to overcome the -1000 penalty.
	for i, ri := range results {
		if !isAbridged(ri.Title) {
			continue
		}
		for j, rj := range results {
			if j >= i {
				break
			}
			if isAbridged(rj.Title) {
				continue
			}
			// rj is non-abridged and ranked above ri (abridged). Good.
			_ = rj
		}
		// If abridged result ri appears before a non-abridged result, it must
		// have overwhelmingly more seeders (seeders - 1000 > other.seeders).
		for j := i + 1; j < len(results); j++ {
			rj := results[j]
			if isAbridged(rj.Title) {
				continue
			}
			if ri.Seeders-1000 <= rj.Seeders {
				t.Errorf("abridged %q (seeders=%d, score=%d) ranks above non-abridged %q (seeders=%d)",
					ri.Title, ri.Seeders, ri.Seeders-1000, rj.Title, rj.Seeders)
			}
		}
	}
}

func TestIntegration_HandlerMissingQuery(t *testing.T) {
	// No Prowlarr call needed — handler rejects before dialling out.
	client := integrationClient(t)
	h := prowlarr.NewHandler(client)

	req := httptest.NewRequest(http.MethodGet, "/api/search", nil)
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// isAbridged mirrors the ranking logic: "abridged" present but not "unabridged".
func isAbridged(title string) bool {
	lower := strings.ToLower(title)
	return strings.Contains(lower, "abridged") && !strings.Contains(lower, "unabridged")
}
