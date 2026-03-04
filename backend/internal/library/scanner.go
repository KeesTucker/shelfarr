package library

import (
	"os"
	"path/filepath"
	"strings"

	"shelfarr/internal/metadata"
)

// audioExts is the set of extensions treated as audio files.
var audioExts = map[string]bool{
	".m4b": true, ".mp3": true, ".aac": true, ".flac": true,
	".ogg": true, ".opus": true, ".wav": true, ".m4a": true,
}

// imageExts is the set of extensions treated as images.
var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// FileInfo groups the file extensions present in a book folder by category.
// Each slice contains unique extensions without the leading dot, e.g. ["m4b"].
type FileInfo struct {
	Audio    []string `json:"audio"`
	Metadata []string `json:"metadata"`
	Images   []string `json:"images"`
	Other    []string `json:"other"`
}

// BookEntry describes one book folder (LibraryDir/Author/Title/).
type BookEntry struct {
	AuthorFolder   string         `json:"author_folder"`
	TitleFolder    string         `json:"title_folder"`
	Path           string         `json:"path"`            // absolute path to title folder
	Metadata       *metadata.Book `json:"metadata"`        // parsed metadata, never nil
	MetadataSource string         `json:"metadata_source"` // "opf" | "abs_json" | "folder"
	NeedsRename    bool           `json:"needs_rename"`
	ExpectedAuthor string         `json:"expected_author"`
	ExpectedTitle  string         `json:"expected_title"`
	Files          FileInfo       `json:"files"`
}

// ScanLibrary walks libraryDir two levels deep (Author/Title) and returns one
// BookEntry per title folder. Non-fatal scan errors are silently skipped.
func ScanLibrary(libraryDir string) ([]BookEntry, error) {
	authorDirs, err := os.ReadDir(libraryDir)
	if err != nil {
		return nil, err
	}

	var entries []BookEntry
	for _, authorDir := range authorDirs {
		if !authorDir.IsDir() {
			continue
		}
		authorPath := filepath.Join(libraryDir, authorDir.Name())
		titleDirs, err := os.ReadDir(authorPath)
		if err != nil {
			continue
		}
		for _, titleDir := range titleDirs {
			if !titleDir.IsDir() {
				continue
			}
			titlePath := filepath.Join(authorPath, titleDir.Name())
			entry := scanBook(titlePath, authorDir.Name(), titleDir.Name())
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func scanBook(path, authorFolder, titleFolder string) BookEntry {
	files := collectFiles(path)

	book, source := resolveMetadata(path, titleFolder, authorFolder, files)

	expectedAuthor := sanitizeName(book.Author)
	expectedTitle := sanitizeName(book.Title)

	return BookEntry{
		AuthorFolder:   authorFolder,
		TitleFolder:    titleFolder,
		Path:           path,
		Metadata:       book,
		MetadataSource: source,
		NeedsRename:    expectedAuthor != authorFolder || expectedTitle != titleFolder,
		ExpectedAuthor: expectedAuthor,
		ExpectedTitle:  expectedTitle,
		Files:          files,
	}
}

// resolveMetadata returns metadata and its source for a book folder.
// Priority: OPF > ABS metadata.json > folder name.
func resolveMetadata(dir, titleFolder, authorFolder string, files FileInfo) (*metadata.Book, string) {
	// 1. Look for .opf files
	opfPaths, _ := filepath.Glob(filepath.Join(dir, "*.opf"))
	if len(opfPaths) > 0 {
		best := metadata.ChooseBestOPF(opfPaths, titleFolder)
		if b, err := metadata.ParseOPF(best); err == nil && (b.Title != "" || b.Author != "") {
			if b.Title == "" {
				b.Title = titleFolder
			}
			if b.Author == "" {
				b.Author = authorFolder
			}
			return b, "opf"
		}
	}

	// 2. ABS metadata.json
	if b, err := metadata.ParseABSMetadata(dir); err == nil && (b.Title != "" || b.Author != "") {
		if b.Title == "" {
			b.Title = titleFolder
		}
		if b.Author == "" {
			b.Author = authorFolder
		}
		return b, "abs_json"
	}

	// 3. Fall back to folder names
	return &metadata.Book{Title: titleFolder, Author: authorFolder}, "folder"
}

// collectFiles scans dir (non-recursively) and buckets file extensions.
func collectFiles(dir string) FileInfo {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return FileInfo{}
	}

	audio := map[string]bool{}
	meta := map[string]bool{}
	images := map[string]bool{}
	other := map[string]bool{}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == "" {
			continue
		}
		extNoDot := ext[1:]
		switch {
		case audioExts[ext]:
			audio[extNoDot] = true
		case ext == ".opf":
			meta[extNoDot] = true
		case ext == ".json":
			meta[extNoDot] = true
		case imageExts[ext]:
			images[extNoDot] = true
		default:
			other[extNoDot] = true
		}
	}

	return FileInfo{
		Audio:    mapKeys(audio),
		Metadata: mapKeys(meta),
		Images:   mapKeys(images),
		Other:    mapKeys(other),
	}
}

func mapKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
