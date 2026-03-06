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
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
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

// searchLibraryItems searches a single ABS library by title, paginating until
// the item is found or all pages are exhausted. Returns the item ID or "".
func (c *Client) searchLibraryItems(ctx context.Context, apiKey, libraryID, title, author string) (string, error) {
	titleLower := strings.ToLower(title)
	authorLower := strings.ToLower(author)
	search := url.QueryEscape(title)

	for page := 0; ; page++ {
		searchURL := fmt.Sprintf("%s/api/libraries/%s/items?search=%s&page=%d",
			c.baseURL, url.PathEscape(libraryID), search, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
		if err != nil {
			return "", fmt.Errorf("build search request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)

		resp, err := c.httpClient.Do(req) //nolint:gosec
		if err != nil {
			return "", fmt.Errorf("search items page %d: %w", page, err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return "", fmt.Errorf("search items page %d: unexpected status %d", page, resp.StatusCode)
		}
		var itemsResp absLibraryItemsResponse
		if err := json.NewDecoder(resp.Body).Decode(&itemsResp); err != nil {
			_ = resp.Body.Close()
			return "", fmt.Errorf("decode items page %d: %w", page, err)
		}
		_ = resp.Body.Close()

		for _, item := range itemsResp.Results {
			if strings.ToLower(item.Media.Metadata.Title) == titleLower &&
				strings.ToLower(item.Media.Metadata.AuthorName) == authorLower {
				return item.ID, nil
			}
		}

		// Stop when this is the last page: no results, or we've seen all items.
		if len(itemsResp.Results) == 0 || itemsResp.Limit <= 0 ||
			(page+1)*itemsResp.Limit >= itemsResp.Total {
			break
		}
	}
	return "", nil
}

// MergeMultiPart calls the ABS merge-multipart tool for the given library item
// ID. ABS processes the merge asynchronously; this returns once the job is queued.
func (c *Client) MergeMultiPart(ctx context.Context, apiKey, itemID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/tools/item/"+url.PathEscape(itemID)+"/merge-multipart", nil)
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
