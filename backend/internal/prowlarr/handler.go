package prowlarr

import (
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"shelfarr/internal/respond"
)

// Result is the normalised search result returned by GET /api/search.
type Result struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Author      string   `json:"author"`
	Narrator    string   `json:"narrator"`
	Tags        []string `json:"tags"`
	Size        int64    `json:"size"`
	Seeders     int      `json:"seeders"`
	Indexer     string   `json:"indexer"`
	PublishDate string   `json:"publishDate"`
	DownloadURL string   `json:"downloadUrl"`
}

// Handler handles /api/search.
type Handler struct {
	client *Client
}

// NewHandler creates a search handler backed by client.
func NewHandler(client *Client) *Handler {
	return &Handler{client: client}
}

// Search handles GET /api/search?q=<query>&type=<audiobook|ebook>.
// Requires JWT (enforced upstream).
func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		respond.Error(w, http.StatusBadRequest, "q parameter is required")
		return
	}

	mediaType := r.URL.Query().Get("type")
	if mediaType != "ebook" {
		mediaType = "audiobook"
	}

	releases, err := h.client.Search(r.Context(), query, mediaType)
	if err != nil {
		slog.Error("prowlarr search", "query", query, "type", mediaType, "err", err) //nolint:gosec
		respond.Error(w, http.StatusBadGateway, "search unavailable")
		return
	}

	respond.JSON(w, http.StatusOK, rank(releases))
}

// ── ranking ───────────────────────────────────────────────────────────────────

func rank(releases []Release) []Result {
	type scored struct {
		result Result
		score  int
	}

	items := make([]scored, 0, len(releases))
	for _, r := range releases {
		tags := extractTags(r.Title)
		title, author, narrator := parseTitle(r.Title)

		score := r.Seeders
		lower := strings.ToLower(r.Title)
		// Deprioritize abridged recordings. Guard against "unabridged" matching.
		if strings.Contains(lower, "abridged") && !strings.Contains(lower, "unabridged") {
			score -= 1000
		}

		pub := ""
		if !r.PublishDate.IsZero() {
			pub = r.PublishDate.Format(time.RFC3339)
		}

		items = append(items, scored{
			result: Result{
				ID:          r.GUID,
				Title:       title,
				Author:      author,
				Narrator:    narrator,
				Tags:        tags,
				Size:        r.Size,
				Seeders:     r.Seeders,
				Indexer:     r.Indexer,
				PublishDate: pub,
				DownloadURL: r.DownloadURL,
			},
			score: score,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	results := make([]Result, len(items))
	for i, item := range items {
		results[i] = item.result
	}
	return results
}

// ── title parsing ─────────────────────────────────────────────────────────────

// knownSuffixes are stripped from the raw torrent title before parsing.
var knownSuffixes = []string{
	"[audiobook]", "[audio book]", "[unabridged]", "(unabridged)",
	"[abridged]", "(abridged)", "[m4b]", "[mp3]", "[flac]", "[aac]",
	"[m4a]", "[ogg]", "[opus]", "[retail]", "[retail audio]",
}

// inlineTagRe matches bracket/paren tags that consist entirely of uppercase
// letters, digits, slashes, and spaces — e.g. [ENG / MP3], [ENG], (FLAC).
// These are language/format annotations, not part of the title or author name.
var inlineTagRe = regexp.MustCompile(`\s*[\[(][A-Z][A-Z0-9 /]*[\])]`)

// extractTags returns the content of all inline tags found in s, with brackets
// stripped — e.g. "[ENG / MP3]" becomes "ENG / MP3".
func extractTags(s string) []string {
	matches := inlineTagRe.FindAllString(s, -1)
	tags := make([]string, 0, len(matches))
	for _, m := range matches {
		m = strings.TrimSpace(m)
		m = strings.Trim(m, "[(])")
		m = strings.TrimSpace(m)
		if m != "" {
			tags = append(tags, m)
		}
	}
	return tags
}

// narratorPhraseRe matches "narrated by X", "read by X", "narrator: X".
var narratorPhraseRe = regexp.MustCompile(`(?i)(?:narrated by|read by|narrator[:\s]+)\s*([A-Za-z][A-Za-z .'-]{2,40})`)

// parseTitle attempts to extract a clean title, author, and narrator from a
// raw audiobook torrent name. The results are best-effort; title always falls
// back to the stripped raw value so it is never empty.
func parseTitle(raw string) (title, author, narrator string) {
	s := stripSuffixes(raw)

	// Extract "narrated by / read by" before dash-splitting so it doesn't
	// interfere with the structure detection.
	if m := narratorPhraseRe.FindStringSubmatch(s); len(m) == 2 {
		narrator = strings.TrimSpace(m[1])
		s = narratorPhraseRe.ReplaceAllString(s, "")
		// Remove any trailing " - " left behind after phrase removal.
		for strings.HasSuffix(s, " - ") || strings.HasSuffix(s, " -") {
			s = strings.TrimSuffix(s, " - ")
			s = strings.TrimSuffix(s, " -")
		}
		s = strings.TrimSpace(s)
	}

	// Split on " - " (with spaces, the conventional audiobook separator).
	parts := splitOnDash(s)

	switch len(parts) {
	case 1:
		// No dashes — try "Title by Author".
		title, author = extractByPattern(parts[0])
	case 2:
		// "Author - Title" or "Title - Author": assume first = author.
		author = parts[0]
		title = parts[1]
	default:
		// 3+ parts: "Author - Title - Narrator" or "Author - Series - Title - Narrator".
		author = parts[0]
		last := parts[len(parts)-1]
		if narrator == "" && looksLikeName(last) {
			narrator = last
			title = strings.Join(parts[1:len(parts)-1], " - ")
		} else {
			title = strings.Join(parts[1:], " - ")
		}
	}

	// Strip year tokens like "(2006)" or "[2006]" from the end of title.
	title = strings.TrimSpace(stripYearSuffix(title))

	if title == "" {
		title = raw
	}
	return title, author, narrator
}

func stripSuffixes(s string) string {
	lower := strings.ToLower(s)
	for _, suffix := range knownSuffixes {
		if strings.HasSuffix(lower, suffix) {
			s = s[:len(s)-len(suffix)]
			lower = strings.ToLower(s)
		}
	}
	// Strip inline language/format tags from anywhere in the string.
	s = inlineTagRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func splitOnDash(s string) []string {
	raw := strings.Split(s, " - ")
	parts := make([]string, 0, len(raw))
	for _, p := range raw {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return []string{s}
	}
	return parts
}

// extractByPattern tries "Title by Author". Returns (title, author) on match,
// or (original, "") if the pattern is not found.
func extractByPattern(s string) (title, author string) {
	lower := strings.ToLower(s)
	idx := strings.Index(lower, " by ")
	if idx < 0 {
		return s, ""
	}
	return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+4:])
}

// nonNameFirstWords are words that narrator names never start with but that
// commonly appear at the front of title segments (articles, format terms, etc.).
var nonNameFirstWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "and": true,
	"book": true, "audio": true, "audiobook": true,
	"unabridged": true, "abridged": true, "complete": true,
}

// looksLikeName is a heuristic: a "name" contains only letters, spaces, dots,
// hyphens, and apostrophes; is 4–50 chars long; has no digits; has 2–4 words;
// and does not begin with a common non-name word.
func looksLikeName(s string) bool {
	if len(s) < 4 || len(s) > 50 {
		return false
	}
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return false
		}
		if r == '[' || r == ']' || r == '(' || r == ')' {
			return false
		}
	}
	words := strings.Fields(s)
	if len(words) < 2 || len(words) > 4 {
		return false
	}
	if nonNameFirstWords[strings.ToLower(words[0])] {
		return false
	}
	return true
}

var yearSuffixRe = regexp.MustCompile(`[\s(\[]*\d{4}[\s)\]]*$`)

func stripYearSuffix(s string) string {
	return yearSuffixRe.ReplaceAllString(s, "")
}
