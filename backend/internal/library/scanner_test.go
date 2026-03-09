package library

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScanLibrary_Empty(t *testing.T) {
	entries, err := ScanLibrary(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestScanLibrary_NonExistent(t *testing.T) {
	_, err := ScanLibrary(filepath.Join(t.TempDir(), "no-such-dir"))
	if err == nil {
		t.Error("expected error for non-existent directory")
	}
}

func TestScanLibrary_WithBooks(t *testing.T) {
	libDir := t.TempDir()
	for _, p := range []string{
		filepath.Join(libDir, "Brandon Sanderson", "The Final Empire"),
		filepath.Join(libDir, "Brandon Sanderson", "The Well of Ascension"),
		filepath.Join(libDir, "Frank Herbert", "Dune"),
	} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestScanLibrary_SkipsFiles(t *testing.T) {
	libDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(libDir, "README.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libDir, "Author", "Title"), 0o755); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (files at author level skipped), got %d", len(entries))
	}
}

func TestScanBook_NoRenameNeeded(t *testing.T) {
	e := scanBook(t.TempDir(), "Brandon Sanderson", "The Final Empire")
	if e.NeedsRename {
		t.Errorf("NeedsRename should be false for clean names; expectedAuthor=%q expectedTitle=%q",
			e.ExpectedAuthor, e.ExpectedTitle)
	}
}

func TestScanBook_TitleNeedsRename(t *testing.T) {
	e := scanBook(t.TempDir(), "Brandon Sanderson", "Mistborn: The Final Empire")
	if !e.NeedsRename {
		t.Errorf("NeedsRename should be true for title with colon; ExpectedTitle=%q", e.ExpectedTitle)
	}
	if e.ExpectedTitle != "Mistborn The Final Empire" {
		t.Errorf("ExpectedTitle=%q; want %q", e.ExpectedTitle, "Mistborn The Final Empire")
	}
}

func TestScanBook_AuthorNeedsRename(t *testing.T) {
	e := scanBook(t.TempDir(), "Author/With/Slash", "Clean Title")
	if !e.NeedsRename {
		t.Error("NeedsRename should be true for author with slashes")
	}
}

func TestScanBook_FieldsPopulated(t *testing.T) {
	dir := t.TempDir()
	e := scanBook(dir, "Frank Herbert", "Dune")
	if e.AuthorFolder != "Frank Herbert" {
		t.Errorf("AuthorFolder=%q", e.AuthorFolder)
	}
	if e.TitleFolder != "Dune" {
		t.Errorf("TitleFolder=%q", e.TitleFolder)
	}
	if e.Path != dir {
		t.Errorf("Path=%q; want %q", e.Path, dir)
	}
	if e.Metadata == nil {
		t.Error("Metadata should not be nil")
	}
}

func TestResolveMetadata_FolderFallback(t *testing.T) {
	book, source := resolveMetadata(t.TempDir(), "My Title", "My Author", FileInfo{})
	if source != "folder" {
		t.Errorf("source=%q; want %q", source, "folder")
	}
	if book.Title != "My Title" {
		t.Errorf("Title=%q; want %q", book.Title, "My Title")
	}
	if book.Author != "My Author" {
		t.Errorf("Author=%q; want %q", book.Author, "My Author")
	}
}

func TestResolveMetadata_OPF(t *testing.T) {
	dir := t.TempDir()
	// Use the same format that EnsureOPF writes.
	opf := `<?xml version="1.0" encoding="UTF-8"?>
<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/" version="2.0" unique-identifier="uid">
  <metadata>
    <dc:identifier id="uid">Frank Herbert - Dune</dc:identifier>
    <dc:title>Dune</dc:title>
    <dc:creator>Frank Herbert</dc:creator>
    <dc:date>1965-01-01</dc:date>
  </metadata>
</package>`
	if err := os.WriteFile(filepath.Join(dir, "dune.opf"), []byte(opf), 0o644); err != nil {
		t.Fatal(err)
	}
	book, source := resolveMetadata(dir, "Dune", "Frank Herbert", FileInfo{})
	if source != "opf" {
		t.Errorf("source=%q; want %q", source, "opf")
	}
	if book.Title != "Dune" {
		t.Errorf("Title=%q; want %q", book.Title, "Dune")
	}
	if book.Author != "Frank Herbert" {
		t.Errorf("Author=%q; want %q", book.Author, "Frank Herbert")
	}
	if book.Year != 1965 {
		t.Errorf("Year=%d; want 1965", book.Year)
	}
}

func TestResolveMetadata_ABSJson(t *testing.T) {
	dir := t.TempDir()
	m := map[string]string{
		"title":         "Dune",
		"authorName":    "Frank Herbert",
		"publishedYear": "1965",
	}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	book, source := resolveMetadata(dir, "Dune", "Frank Herbert", FileInfo{})
	if source != "abs_json" {
		t.Errorf("source=%q; want %q", source, "abs_json")
	}
	if book.Title != "Dune" {
		t.Errorf("Title=%q; want %q", book.Title, "Dune")
	}
	if book.Author != "Frank Herbert" {
		t.Errorf("Author=%q; want %q", book.Author, "Frank Herbert")
	}
	if book.Year != 1965 {
		t.Errorf("Year=%d; want 1965", book.Year)
	}
}

func TestResolveMetadata_OPFMissingTitleFallsBackToFolder(t *testing.T) {
	dir := t.TempDir()
	// OPF with empty title/author — should fall back to folder names
	opf := `<?xml version="1.0"?><package><metadata></metadata></package>`
	if err := os.WriteFile(filepath.Join(dir, "empty.opf"), []byte(opf), 0o644); err != nil {
		t.Fatal(err)
	}
	book, source := resolveMetadata(dir, "Folder Title", "Folder Author", FileInfo{})
	// Empty OPF → falls through to folder
	if source == "opf" && (book.Title == "" || book.Author == "") {
		t.Error("empty OPF should not leave title or author blank")
	}
	_ = source
	if book.Title == "" {
		t.Error("Title should never be empty")
	}
	if book.Author == "" {
		t.Error("Author should never be empty")
	}
}

func TestCollectFiles_Categories(t *testing.T) {
	dir := t.TempDir()
	for name, data := range map[string]string{
		"audiobook.m4b": "audio",
		"bonus.mp3":     "audio2",
		"metadata.opf":  "opf",
		"info.json":     "json",
		"cover.jpg":     "image",
		"readme.txt":    "text",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fi := collectFiles(dir)
	if len(fi.Audio) != 2 {
		t.Errorf("Audio=%v; want 2 items", fi.Audio)
	}
	if len(fi.Metadata) != 2 {
		t.Errorf("Metadata=%v; want 2 items (opf+json)", fi.Metadata)
	}
	if len(fi.Images) != 1 {
		t.Errorf("Images=%v; want 1 item", fi.Images)
	}
	if len(fi.Other) != 1 {
		t.Errorf("Other=%v; want 1 item", fi.Other)
	}
}

func TestCollectFiles_SkipsDirs(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi := collectFiles(dir)
	if len(fi.Audio) != 1 {
		t.Errorf("Audio=%v; want 1 (subdir should be skipped)", fi.Audio)
	}
}

func TestCollectFiles_NonExistent(t *testing.T) {
	fi := collectFiles(filepath.Join(t.TempDir(), "no-such-dir"))
	if fi.Audio != nil || fi.Metadata != nil || fi.Images != nil || fi.Other != nil {
		t.Error("expected all-nil FileInfo for non-existent dir")
	}
}

func TestCollectFiles_SkipsNoExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "noext"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	fi := collectFiles(dir)
	if len(fi.Other) != 0 {
		t.Errorf("file with no extension should be skipped; Other=%v", fi.Other)
	}
}

// ── IsMultiPart / AudioCount ──────────────────────────────────────────────────

func TestScanBook_IsMultiPartFalseForSingleAudioFile(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "book.m4b"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].IsMultiPart {
		t.Error("IsMultiPart should be false for a single audio file")
	}
}

func TestScanBook_IsMultiPartTrueForMultipleFiles(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"part1.mp3", "part2.mp3"} {
		if err := os.WriteFile(filepath.Join(titlePath, name), []byte("audio"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatal(err)
	}
	if !entries[0].IsMultiPart {
		t.Error("IsMultiPart should be true for multiple audio files")
	}
}

func TestScanBook_IsMultiPartTrueForFilesInSubdirs(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(filepath.Join(titlePath, "disc1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(titlePath, "disc2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "disc1", "ch1.mp3"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "disc2", "ch1.mp3"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatal(err)
	}
	if !entries[0].IsMultiPart {
		t.Error("IsMultiPart should be true when audio files are spread across subdirs")
	}
}

func TestScanBook_IsMultiPartFalseForNoAudioFiles(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "cover.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].IsMultiPart {
		t.Error("IsMultiPart should be false when there are no audio files")
	}
}

func TestCollectFiles_AudioCount(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.mp3", "b.m4b"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// file in subdir — should still count
	if err := os.WriteFile(filepath.Join(sub, "c.flac"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// non-audio file — should not count
	if err := os.WriteFile(filepath.Join(dir, "cover.jpg"), []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fi := collectFiles(dir); fi.AudioCount != 3 {
		t.Errorf("AudioCount = %d; want 3", fi.AudioCount)
	}
}

// ── NeedsEncode ───────────────────────────────────────────────────────────────

func TestNeedsEncode_SingleM4B(t *testing.T) {
	if needsEncode(FileInfo{AudioCount: 1, Audio: []string{"m4b"}}) {
		t.Error("single M4B should not need encoding")
	}
}

func TestNeedsEncode_SingleMP3(t *testing.T) {
	if !needsEncode(FileInfo{AudioCount: 1, Audio: []string{"mp3"}}) {
		t.Error("single MP3 should need encoding")
	}
}

func TestNeedsEncode_MultipleM4Bs(t *testing.T) {
	if !needsEncode(FileInfo{AudioCount: 2, Audio: []string{"m4b"}}) {
		t.Error("multiple M4Bs should need encoding (merge into one)")
	}
}

func TestNeedsEncode_MultipleMP3s(t *testing.T) {
	if !needsEncode(FileInfo{AudioCount: 2, Audio: []string{"mp3"}}) {
		t.Error("multiple MP3s should need encoding")
	}
}

func TestNeedsEncode_NoAudio(t *testing.T) {
	if needsEncode(FileInfo{AudioCount: 0}) {
		t.Error("no audio files should not need encoding")
	}
}

func TestScanBook_NeedsEncodeFalseForSingleM4B(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "book.m4b"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatal(err)
	}
	if entries[0].NeedsEncode {
		t.Error("NeedsEncode should be false for a single M4B file")
	}
}

func TestScanBook_NeedsEncodeTrueForSingleMP3(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Author", "Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "book.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatal(err)
	}
	if !entries[0].NeedsEncode {
		t.Error("NeedsEncode should be true for a single non-M4B audio file")
	}
}

func TestHasNestedDirs_True(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hasNestedDirs(dir) {
		t.Error("expected true when subdir exists")
	}
}

func TestHasNestedDirs_False(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ch.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	if hasNestedDirs(dir) {
		t.Error("expected false when no subdirs exist")
	}
}

func TestScanBook_NeedsFlatWhenSubdirsPresent(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Some Author", "Some Book")
	if err := os.MkdirAll(filepath.Join(titlePath, "disc1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "disc1", "ch.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].NeedsFlat {
		t.Error("NeedsFlat should be true when title folder contains subdirectories")
	}
}

func TestScanBook_NeedsFlatFalseWhenFlat(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Some Author", "Some Book")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "ch.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ScanLibrary(libDir)
	if err != nil {
		t.Fatalf("ScanLibrary: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].NeedsFlat {
		t.Error("NeedsFlat should be false when all files are at top level")
	}
}
