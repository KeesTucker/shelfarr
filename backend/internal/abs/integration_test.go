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
//	ABS_URL=http://... (default: http://10.10.10.2:13378)

import (
	"context"
	"errors"
	"os"
	"testing"

	"shelfarr/internal/abs"
)

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
