package metadata

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ── OPF parsing ───────────────────────────────────────────────────────────────

// opfParsedMeta is the minimal shape we need from an OPF package document.
type opfParsedMeta struct {
	Titles   []string `xml:"title"`
	Creators []string `xml:"creator"`
	Dates    []string `xml:"date"`
}

type opfDoc struct {
	Metadata opfParsedMeta `xml:"metadata"`
}

// ParseOPF reads an OPF file and returns the book metadata it contains.
func ParseOPF(path string) (*Book, error) {
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("open opf: %w", err)
	}
	defer func() { _ = f.Close() }()

	var doc opfDoc
	if err := xml.NewDecoder(f).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode opf: %w", err)
	}

	b := &Book{}
	if len(doc.Metadata.Titles) > 0 {
		b.Title = strings.TrimSpace(doc.Metadata.Titles[0])
	}
	if len(doc.Metadata.Creators) > 0 {
		b.Author = strings.TrimSpace(doc.Metadata.Creators[0])
	}
	if len(doc.Metadata.Dates) > 0 {
		// dc:date may be "2006-01-01" or just "2006"
		part := strings.TrimSpace(doc.Metadata.Dates[0])
		if len(part) >= 4 {
			part = part[:4]
		}
		if y, err := strconv.Atoi(part); err == nil {
			b.Year = y
		}
	}
	return b, nil
}

// ChooseBestOPF returns the path from paths that has the most word overlap
// with title. Comparison is case-insensitive. First path wins ties.
func ChooseBestOPF(paths []string, title string) string {
	if len(paths) == 0 {
		return ""
	}
	titleWords := wordSet(strings.ToLower(title))
	best, bestScore := paths[0], -1
	for _, p := range paths {
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
		score := 0
		for w := range wordSet(base) {
			if titleWords[w] {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = p, score
		}
	}
	return best
}

func wordSet(s string) map[string]bool {
	m := make(map[string]bool)
	for _, w := range strings.Fields(s) {
		m[w] = true
	}
	return m
}

// ── OPF patching ──────────────────────────────────────────────────────────────

var (
	reTitle      = regexp.MustCompile(`(?i)(<dc:title[^>]*>)[^<]*(</dc:title>)`)
	reCreator    = regexp.MustCompile(`(?i)(<dc:creator[^>]*>)[^<]*(</dc:creator>)`)
	reIdentifier = regexp.MustCompile(`(?i)(<dc:identifier[^>]*>)[^<]*(</dc:identifier>)`)
	reDate       = regexp.MustCompile(`(?i)(<dc:date[^>]*>)[^<]*(</dc:date>)`)
)

// PatchOPF edits the dc:title, dc:creator, dc:identifier, and dc:date elements
// of an existing OPF file in-place, preserving all other content.
// Returns an error if any required tag is missing (caller should fall back to regen).
func PatchOPF(path string, book *Book) error {
	raw, err := os.ReadFile(path) //nolint:gosec
	if err != nil {
		return fmt.Errorf("read opf: %w", err)
	}

	body := string(raw)
	escaped := func(s string) string { return xmlEscape(s) }

	if !reTitle.MatchString(body) {
		return fmt.Errorf("patch opf: <dc:title> not found in %s", path)
	}
	if !reCreator.MatchString(body) {
		return fmt.Errorf("patch opf: <dc:creator> not found in %s", path)
	}

	body = reTitle.ReplaceAllString(body, "${1}"+escaped(book.Title)+"${2}")
	body = reCreator.ReplaceAllString(body, "${1}"+escaped(book.Author)+"${2}")
	body = reIdentifier.ReplaceAllString(body, "${1}"+escaped(book.Author+" - "+book.Title)+"${2}")

	if book.Year > 0 {
		dateVal := fmt.Sprintf("%d-01-01", book.Year)
		if reDate.MatchString(body) {
			body = reDate.ReplaceAllString(body, "${1}"+dateVal+"${2}")
		}
		// If no dc:date existed we leave it absent rather than trying to insert it.
	}

	return os.WriteFile(path, []byte(body), 0o644) //nolint:gosec
}

// xmlEscape escapes the five predefined XML entities.
func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// ── ABS metadata.json ─────────────────────────────────────────────────────────

type absMetadataJSON struct {
	Title         string `json:"title"`
	AuthorName    string `json:"authorName"`
	PublishedYear string `json:"publishedYear"`
}

// ParseABSMetadata reads an AudioBookShelf metadata.json from dir and returns
// a Book. Returns an error if the file doesn't exist or can't be parsed.
func ParseABSMetadata(dir string) (*Book, error) {
	path := filepath.Join(dir, "metadata.json")
	f, err := os.Open(path) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("open abs metadata: %w", err)
	}
	defer func() { _ = f.Close() }()

	var m absMetadataJSON
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return nil, fmt.Errorf("decode abs metadata: %w", err)
	}

	b := &Book{
		Title:  strings.TrimSpace(m.Title),
		Author: strings.TrimSpace(m.AuthorName),
	}
	if y, err := strconv.Atoi(strings.TrimSpace(m.PublishedYear)); err == nil {
		b.Year = y
	}
	return b, nil
}
