//go:build integration

package abs_test

// Integration tests that talk to a real AudioBookShelf instance.
//
// Run with:
//
//	ABS_TEST_USERNAME=admin ABS_TEST_PASSWORD=<pw> go test -tags integration -v ./internal/abs/
//
// Optional overrides:
//
//	ABS_URL=http://...          (default: http://10.10.10.2:13378)
//	ABS_API_KEY=<token>         required for library / merge tests
//	ABS_TEST_ITEM_ID=<item-id>  enables the real MergeMultiPart test

import (
	"context"
	"errors"
	"os"
	"testing"

	"shelfarr/internal/abs"
)

// integrationAPIKey returns the ABS API key from the environment, skipping the
// test if it is not set.
func integrationAPIKey(t *testing.T) string {
	t.Helper()
	key := os.Getenv("ABS_API_KEY")
	if key == "" {
		t.Skip("ABS_API_KEY not set; skipping integration test")
	}
	return key
}

// integrationClient creates a real ABS client from env vars.
// The test is skipped if ABS_TEST_USERNAME or ABS_TEST_PASSWORD is not set.
func integrationClient(t *testing.T) (*abs.Client, string, string) {
	t.Helper()
	username := os.Getenv("ABS_TEST_USERNAME")
	password := os.Getenv("ABS_TEST_PASSWORD")
	if username == "" || password == "" {
		t.Skip("ABS_TEST_USERNAME / ABS_TEST_PASSWORD not set; skipping integration test")
	}
	baseURL := os.Getenv("ABS_URL")
	if baseURL == "" {
		baseURL = "http://10.10.10.2:13378"
	}
	return abs.New(baseURL), username, password
}

func TestIntegration_LoginSuccess(t *testing.T) {
	client, username, password := integrationClient(t)

	user, err := client.Login(context.Background(), username, password)
	if err != nil {
		t.Fatalf("Login(%q): %v", username, err)
	}

	if user.ID == "" {
		t.Error("expected non-empty user ID")
	}
	if user.Username == "" {
		t.Error("expected non-empty username")
	}
	if user.Type == "" {
		t.Error("expected non-empty user type")
	}
	if role := user.Role(); role != "admin" && role != "user" {
		t.Errorf("Role() = %q; want admin or user", role)
	}

	t.Logf("logged in: id=%q username=%q type=%q role=%q",
		user.ID, user.Username, user.Type, user.Role())
}

func TestIntegration_LoginInvalidPassword(t *testing.T) {
	client, username, _ := integrationClient(t)

	_, err := client.Login(context.Background(), username, "definitely-wrong-password-xyz")
	if !errors.Is(err, abs.ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

func TestIntegration_LoginUnknownUser(t *testing.T) {
	client, _, _ := integrationClient(t)

	_, err := client.Login(context.Background(), "user-that-does-not-exist-xyz", "pw")
	if !errors.Is(err, abs.ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

// ── Library / merge ───────────────────────────────────────────────────────────

// TestIntegration_FindLibraryItemByTitleAuthor_ReachesABS verifies that the
// /api/libraries endpoint is reachable and parseable. It does not assert that
// a specific item is found — that depends on library contents.
func TestIntegration_FindLibraryItemByTitleAuthor_ReachesABS(t *testing.T) {
	client, _, _ := integrationClient(t)
	apiKey := integrationAPIKey(t)

	// A search that is unlikely to match anything — we only care that the call
	// succeeds without a network or parse error.
	itemID, err := client.FindLibraryItemByTitleAuthor(
		context.Background(), apiKey,
		"__integration_test_nonexistent_title__",
		"__integration_test_nonexistent_author__",
	)
	if err != nil {
		t.Fatalf("FindLibraryItemByTitleAuthor: %v", err)
	}
	if itemID != "" {
		t.Logf("unexpectedly found item %q (library contains test data?)", itemID)
	}
}

// TestIntegration_MergeMultiPart_InvalidItem verifies that calling merge with a
// non-existent item ID returns an error rather than silently succeeding.
func TestIntegration_MergeMultiPart_InvalidItem(t *testing.T) {
	client, _, _ := integrationClient(t)
	apiKey := integrationAPIKey(t)

	err := client.MergeMultiPart(context.Background(), apiKey, "li_nonexistent_integration_test")
	if err == nil {
		t.Fatal("expected error for non-existent item ID, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// TestIntegration_MergeMultiPart_RealItem triggers an actual ABS merge for the
// item ID given in ABS_TEST_ITEM_ID. Skipped unless that var is set.
// WARNING: this enqueues a real merge job in ABS — only use it against test data.
func TestIntegration_MergeMultiPart_RealItem(t *testing.T) {
	client, _, _ := integrationClient(t)
	apiKey := integrationAPIKey(t)

	itemID := os.Getenv("ABS_TEST_ITEM_ID")
	if itemID == "" {
		t.Skip("ABS_TEST_ITEM_ID not set; skipping real merge test")
	}

	if err := client.MergeMultiPart(context.Background(), apiKey, itemID); err != nil {
		t.Fatalf("MergeMultiPart(%q): %v", itemID, err)
	}
	t.Logf("merge job queued for item %q", itemID)
}
