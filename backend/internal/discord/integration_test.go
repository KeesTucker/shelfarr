//go:build integration

package discord_test

// Integration tests that send real messages to a Discord webhook.
//
// Run with:
//
//	DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/... go test -tags integration -v ./internal/discord/
//
// The test is skipped when DISCORD_WEBHOOK_URL is not set. When run, a visible
// message will appear in the configured Discord channel — use a test channel.

import (
	"context"
	"os"
	"testing"
	"time"

	"shelfarr/internal/discord"
	"shelfarr/internal/metadata"
)

func webhookURL(t *testing.T) string {
	t.Helper()
	u := os.Getenv("DISCORD_WEBHOOK_URL")
	if u == "" {
		t.Skip("DISCORD_WEBHOOK_URL not set; skipping integration test")
	}
	return u
}

// ── Send ──────────────────────────────────────────────────────────────────────

// TestIntegration_Send posts a plain message and verifies no error is returned.
func TestIntegration_Send(t *testing.T) {
	url := webhookURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := discord.Send(ctx, url, "📡 shelfarr integration test — plain Send (ignore this message)")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	t.Log("plain message sent successfully")
}

// ── NotifyComplete ─────────────────────────────────────────────────────────────

// TestIntegration_NotifyComplete sends a realistic download-complete notification.
func TestIntegration_NotifyComplete(t *testing.T) {
	url := webhookURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	book := &metadata.Book{
		Title:    "The Final Empire",
		Author:   "Brandon Sanderson",
		Narrator: "Michael Kramer",
		Year:     2006,
		Series:   "Mistborn",
	}
	err := discord.NotifyComplete(ctx, url, book, "testuser",
		"/audiobooks/Brandon Sanderson/The Final Empire (2006)")
	if err != nil {
		t.Fatalf("NotifyComplete: %v", err)
	}
	t.Log("complete notification sent successfully")
}

// TestIntegration_NotifyComplete_NoNarrator verifies the narrator line is
// omitted gracefully when the book has no narrator metadata.
func TestIntegration_NotifyComplete_NoNarrator(t *testing.T) {
	url := webhookURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	book := &metadata.Book{
		Title:  "Dune",
		Author: "Frank Herbert",
		Year:   1965,
	}
	err := discord.NotifyComplete(ctx, url, book, "testuser", "/audiobooks/Frank Herbert/Dune (1965)")
	if err != nil {
		t.Fatalf("NotifyComplete (no narrator): %v", err)
	}
	t.Log("complete notification (no narrator) sent successfully")
}

// ── NotifyFailed ──────────────────────────────────────────────────────────────

// TestIntegration_NotifyFailed sends a realistic download-failed notification.
func TestIntegration_NotifyFailed(t *testing.T) {
	url := webhookURL(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := discord.NotifyFailed(ctx, url,
		"Mistborn: The Final Empire", "Brandon Sanderson",
		"testuser", "qBit torrent stalled after 2 hours")
	if err != nil {
		t.Fatalf("NotifyFailed: %v", err)
	}
	t.Log("failed notification sent successfully")
}

// ── edge cases ────────────────────────────────────────────────────────────────

// TestIntegration_EmptyURL_NoOp confirms that an empty webhook URL is always a
// no-op regardless of what message would have been sent.
func TestIntegration_EmptyURL_NoOp(t *testing.T) {
	// This sub-test runs even without DISCORD_WEBHOOK_URL set.
	ctx := context.Background()

	if err := discord.Send(ctx, "", "should not send"); err != nil {
		t.Errorf("Send with empty URL: %v", err)
	}

	book := &metadata.Book{Title: "T", Author: "A"}
	if err := discord.NotifyComplete(ctx, "", book, "u", "/p"); err != nil {
		t.Errorf("NotifyComplete with empty URL: %v", err)
	}
	if err := discord.NotifyFailed(ctx, "", "T", "A", "u", "reason"); err != nil {
		t.Errorf("NotifyFailed with empty URL: %v", err)
	}
}
