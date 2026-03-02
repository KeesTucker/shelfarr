// Package discord sends notifications to a Discord channel via an incoming
// webhook URL. All functions are no-ops when webhookURL is empty.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"bookarr/internal/metadata"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

type webhookPayload struct {
	Content string `json:"content"`
}

// Send posts content as a plain-text message to a Discord webhook.
// Returns nil immediately if webhookURL is empty.
func Send(ctx context.Context, webhookURL, content string) error {
	if webhookURL == "" {
		return nil
	}

	payload, err := json.Marshal(webhookPayload{Content: content})
	if err != nil {
		return fmt.Errorf("discord: marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL,
		bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("discord: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("discord: send: %w", err)
	}
	defer resp.Body.Close()

	// Discord returns 204 No Content on success (no wait parameter).
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// NotifyComplete sends a download-complete notification with book metadata,
// the requesting username, and the final library path.
func NotifyComplete(ctx context.Context, webhookURL string, book *metadata.Book, requestedBy, finalPath string) error {
	narrator := ""
	if book.Narrator != "" {
		narrator = "\nNarrator: " + book.Narrator
	}
	msg := fmt.Sprintf(
		"📚 Download complete: **%s** by %s%s\nRequested by: %s\nSaved to: %s",
		book.Title, book.Author, narrator, requestedBy, finalPath,
	)
	return Send(ctx, webhookURL, msg)
}

// NotifyFailed sends a download-failed notification with a human-readable
// error reason and the requesting username.
func NotifyFailed(ctx context.Context, webhookURL, title, author, requestedBy, reason string) error {
	msg := fmt.Sprintf(
		"❌ Download failed: **%s** by %s\nError: %s\nRequested by: %s",
		title, author, reason, requestedBy,
	)
	return Send(ctx, webhookURL, msg)
}
