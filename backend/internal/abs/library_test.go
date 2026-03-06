package abs_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"shelfarr/internal/abs"
)

// fakeLibraryServer builds a test server from a mux, closing it on test cleanup.
func fakeLibraryServer(t *testing.T, mux *http.ServeMux) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// librariesResponse builds the JSON body for GET /api/libraries.
func librariesResponse(libs ...map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{"libraries": libs})
	return b
}

// itemsResponse builds the JSON body for GET /api/libraries/:id/items.
func itemsResponse(items ...map[string]any) []byte {
	b, _ := json.Marshal(map[string]any{"results": items})
	return b
}

func bookItem(id, title, author string) map[string]any {
	return map[string]any{
		"id": id,
		"media": map[string]any{
			"metadata": map[string]any{
				"title":      title,
				"authorName": author,
			},
		},
	}
}

// ── FindLibraryItemByTitleAuthor ──────────────────────────────────────────────

func TestFindLibraryItemByTitleAuthor_Found(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/libraries", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write(librariesResponse(map[string]any{"id": "lib1", "mediaType": "book"}))
	})
	mux.HandleFunc("GET /api/libraries/lib1/items", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(itemsResponse(bookItem("li_abc", "Dune", "Frank Herbert")))
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	itemID, err := client.FindLibraryItemByTitleAuthor(context.Background(), "test-key", "Dune", "Frank Herbert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if itemID != "li_abc" {
		t.Errorf("itemID: want li_abc, got %q", itemID)
	}
}

func TestFindLibraryItemByTitleAuthor_NotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/libraries", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(librariesResponse(map[string]any{"id": "lib1", "mediaType": "book"}))
	})
	mux.HandleFunc("GET /api/libraries/lib1/items", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(itemsResponse())
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	itemID, err := client.FindLibraryItemByTitleAuthor(context.Background(), "key", "Unknown Book", "Nobody")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if itemID != "" {
		t.Errorf("expected empty itemID for not-found book, got %q", itemID)
	}
}

func TestFindLibraryItemByTitleAuthor_CaseInsensitive(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/libraries", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(librariesResponse(map[string]any{"id": "lib1", "mediaType": "book"}))
	})
	mux.HandleFunc("GET /api/libraries/lib1/items", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(itemsResponse(bookItem("li_xyz", "DUNE", "FRANK HERBERT")))
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	itemID, err := client.FindLibraryItemByTitleAuthor(context.Background(), "key", "dune", "frank herbert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if itemID != "li_xyz" {
		t.Errorf("expected case-insensitive match; got %q", itemID)
	}
}

func TestFindLibraryItemByTitleAuthor_SkipsPodcastLibraries(t *testing.T) {
	podcastCalled := false
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/libraries", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(librariesResponse(map[string]any{"id": "pod1", "mediaType": "podcast"}))
	})
	mux.HandleFunc("GET /api/libraries/pod1/items", func(w http.ResponseWriter, _ *http.Request) {
		podcastCalled = true
		w.Write(itemsResponse(bookItem("li_pod", "Dune", "Frank Herbert")))
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	itemID, err := client.FindLibraryItemByTitleAuthor(context.Background(), "key", "Dune", "Frank Herbert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if podcastCalled {
		t.Error("podcast library items endpoint should not have been called")
	}
	if itemID != "" {
		t.Errorf("expected empty itemID when only podcast libraries present; got %q", itemID)
	}
}

func TestFindLibraryItemByTitleAuthor_SearchesMultipleLibraries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/libraries", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(librariesResponse(
			map[string]any{"id": "lib1", "mediaType": "book"},
			map[string]any{"id": "lib2", "mediaType": "book"},
		))
	})
	mux.HandleFunc("GET /api/libraries/lib1/items", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(itemsResponse()) // not found in lib1
	})
	mux.HandleFunc("GET /api/libraries/lib2/items", func(w http.ResponseWriter, _ *http.Request) {
		w.Write(itemsResponse(bookItem("li_found", "Dune", "Frank Herbert")))
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	itemID, err := client.FindLibraryItemByTitleAuthor(context.Background(), "key", "Dune", "Frank Herbert")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if itemID != "li_found" {
		t.Errorf("expected item from lib2; got %q", itemID)
	}
}

func TestFindLibraryItemByTitleAuthor_LibrariesHTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/libraries", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	_, err := client.FindLibraryItemByTitleAuthor(context.Background(), "key", "Dune", "Frank Herbert")
	if err == nil {
		t.Fatal("expected error for 500 on /api/libraries")
	}
}

// ── MergeMultiPart ────────────────────────────────────────────────────────────

func TestMergeMultiPart_Success(t *testing.T) {
	var gotItemID string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tools/item/{id}/merge-multipart", func(w http.ResponseWriter, r *http.Request) {
		gotItemID = r.PathValue("id")
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	if err := client.MergeMultiPart(context.Background(), "test-key", "li_abc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotItemID != "li_abc" {
		t.Errorf("item ID: want li_abc, got %q", gotItemID)
	}
}

func TestMergeMultiPart_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tools/item/{id}/merge-multipart", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)

	if err := client.MergeMultiPart(context.Background(), "key", "li_abc"); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestMergeMultiPart_SendsAuthHeader(t *testing.T) {
	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tools/item/{id}/merge-multipart", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	})

	client := abs.New(fakeLibraryServer(t, mux).URL)
	_ = client.MergeMultiPart(context.Background(), "my-api-key", "li_x")

	if gotAuth != "Bearer my-api-key" {
		t.Errorf("Authorization: want %q, got %q", "Bearer my-api-key", gotAuth)
	}
}
