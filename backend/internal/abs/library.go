package abs

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

type absLibrary struct {
	ID        string `json:"id"`
	MediaType string `json:"mediaType"`
}

type absLibrariesResponse struct {
	Libraries []absLibrary `json:"libraries"`
}

type absLibraryItemMetadata struct {
	Title      string `json:"title"`
	AuthorName string `json:"authorName"`
}

type absLibraryItemMedia struct {
	Metadata absLibraryItemMetadata `json:"metadata"`
}

type absLibraryItem struct {
	ID    string              `json:"id"`
	Path  string              `json:"path"`
	Media absLibraryItemMedia `json:"media"`
}

type absLibraryItemsResponse struct {
	Results []absLibraryItem `json:"results"`
}

// FindLibraryItemByTitleAuthor searches all ABS book libraries for an item
// whose title and author match (case-insensitive). Returns the item ID, or ""
// if not found in any library.
func (c *Client) FindLibraryItemByTitleAuthor(ctx context.Context, apiKey, title, author string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/libraries", nil)
	if err != nil {
		return "", fmt.Errorf("abs: build libraries request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("abs: get libraries: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("abs: get libraries: unexpected status %d", resp.StatusCode)
	}

	var libsResp absLibrariesResponse
	if err := json.NewDecoder(resp.Body).Decode(&libsResp); err != nil {
		return "", fmt.Errorf("abs: decode libraries: %w", err)
	}

	for _, lib := range libsResp.Libraries {
		if lib.MediaType != "book" {
			continue
		}
		itemID, err := c.searchLibraryItems(ctx, apiKey, lib.ID, title, author)
		if err != nil {
			slog.Warn("abs: search library items", "library_id", lib.ID, "err", err)
			continue
		}
		if itemID != "" {
			return itemID, nil
		}
	}
	return "", nil
}

// searchLibraryItems searches a single ABS library by title and matches the
// result against the expected author. Returns the item ID or "".
func (c *Client) searchLibraryItems(ctx context.Context, apiKey, libraryID, title, author string) (string, error) {
	searchURL := c.baseURL + "/api/libraries/" + libraryID + "/items?search=" + url.QueryEscape(title)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return "", fmt.Errorf("build search request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("search items: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("search items: unexpected status %d", resp.StatusCode)
	}

	var itemsResp absLibraryItemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&itemsResp); err != nil {
		return "", fmt.Errorf("decode items: %w", err)
	}

	titleLower := strings.ToLower(title)
	authorLower := strings.ToLower(author)
	for _, item := range itemsResp.Results {
		if strings.ToLower(item.Media.Metadata.Title) == titleLower &&
			strings.ToLower(item.Media.Metadata.AuthorName) == authorLower {
			return item.ID, nil
		}
	}
	return "", nil
}

// MergeMultiPart calls the ABS merge-multipart tool for the given library item
// ID. ABS processes the merge asynchronously; this returns once the job is queued.
func (c *Client) MergeMultiPart(ctx context.Context, apiKey, itemID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/tools/item/"+itemID+"/merge-multipart", nil)
	if err != nil {
		return fmt.Errorf("abs: build merge request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := c.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("abs: merge multipart: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("abs: merge multipart: unexpected status %d", resp.StatusCode)
	}
	return nil
}
