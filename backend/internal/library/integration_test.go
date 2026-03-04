//go:build integration

package library_test

// Integration tests for the library file mover that exercise real filesystem
//// operations. Unlike unit tests (same package), these tests can only access the
//// public API — internal fields such as pollInterval are not set, so files must
//// already exist in the watch directory before Move() is called (the immediate
//// check at the start of waitForFile returns without waiting for a tick).
//
// Run with:
//
//	go test -tags integration -v ./internal/library/
//
// Optional env var overrides:
//
//	WATCH_DIR=<path>    (default: per-test temp dir)
//	LIBRARY_DIR=<path>  (default: per-test temp dir)

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shelfarr/internal/library"
	"shelfarr/internal/metadata"
)

func watchDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("WATCH_DIR"); d != "" {
		return d
	}
	return t.TempDir()
}

func libraryDir(t *testing.T) string {
	t.Helper()
	if d := os.Getenv("LIBRARY_DIR"); d != "" {
		return d
	}
	return t.TempDir()
}

// TestIntegration_Move_SingleFile verifies that a single-file "torrent" is
// moved to the correct Author/Title (Year) path in the library directory.
func TestIntegration_Move_SingleFile(t *testing.T) {
	wd := watchDir(t)
	ld := libraryDir(t)

	torrentName := "brandon-sanderson-mistborn.mp3"
	if err := os.WriteFile(filepath.Join(wd, torrentName), []byte("audio data"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson", Year: 2006}
	m := library.New(wd, ld, 30*time.Second)

	finalPath, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	t.Logf("moved to: %s", finalPath)

	// File must be at the destination.
	if _, err := os.Stat(finalPath); err != nil {
		t.Errorf("final path does not exist: %v", err)
	}
	// Path must include Author and Title components (year is not appended).
	if !strings.Contains(finalPath, "Brandon Sanderson") {
		t.Errorf("finalPath=%q; expected to contain author name", finalPath)
	}
	if !strings.Contains(finalPath, "The Final Empire") {
		t.Errorf("finalPath=%q; expected to contain title", finalPath)
	}
}

// TestIntegration_Move_Directory verifies that a directory torrent (multi-file)
// is moved recursively and all contents are preserved.
func TestIntegration_Move_Directory(t *testing.T) {
	wd := watchDir(t)
	ld := libraryDir(t)

	torrentName := "Mistborn The Final Empire - Brandon Sanderson"
	src := filepath.Join(wd, torrentName)
	if err := os.MkdirAll(filepath.Join(src, "disc1"), 0o755); err != nil {
		t.Fatal(err)
	}

	// All chapter files land flat in destDir regardless of subdirectory nesting
	// in the source torrent (linkFlat strips subdirectory structure).
	files := []struct{ rel, content string }{
		{"chapter01.mp3", "audio ch1"},
		{"chapter02.mp3", "audio ch2"},
		{"chapter03.mp3", "audio ch3"}, // was in disc1/ — flattened by linkFlat
	}
	// Write source files; chapter03.mp3 lives under disc1/ in the torrent.
	srcFiles := []struct{ rel, content string }{
		{"chapter01.mp3", "audio ch1"},
		{"chapter02.mp3", "audio ch2"},
		{filepath.Join("disc1", "chapter03.mp3"), "audio ch3"},
	}
	for _, f := range srcFiles {
		if err := os.WriteFile(filepath.Join(src, f.rel), []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	book := &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson", Year: 2006}
	m := library.New(wd, ld, 30*time.Second)

	finalPath, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	t.Logf("moved to: %s", finalPath)

	// Every file must be directly in finalPath (flat) with its content intact.
	for _, f := range files {
		p := filepath.Join(finalPath, f.rel)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("file %q not found at destination: %v", f.rel, err)
			continue
		}
		if string(data) != f.content {
			t.Errorf("file %q content=%q; want %q", f.rel, data, f.content)
		}
	}

	// Source directory is kept (hard-link semantics; qBit manages its own cleanup).
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source directory should still exist after link: %v", err)
	}
}

// TestIntegration_Move_FolderNamingSanitisation checks that characters invalid
// in Linux paths are stripped from the resolved author and title.
func TestIntegration_Move_FolderNamingSanitisation(t *testing.T) {
	wd := watchDir(t)
	ld := libraryDir(t)

	torrentName := "test-sanitise-torrent"
	if err := os.WriteFile(filepath.Join(wd, torrentName), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Colon and slash are invalid in Linux path components.
	book := &metadata.Book{
		Title:  "Title: With Colon",
		Author: "Author/With/Slash",
		Year:   2024,
	}
	m := library.New(wd, ld, 30*time.Second)

	finalPath, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	t.Logf("moved to: %s", finalPath)

	// The path must not contain raw colons (after the drive letter on Windows) or
	// extra slashes introduced by the author name.
	rel, err := filepath.Rel(ld, finalPath)
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if strings.Contains(rel, ":") {
		t.Errorf("relative path contains colon: %q", rel)
	}
	// The directory must actually exist.
	if _, err := os.Stat(finalPath); err != nil {
		t.Errorf("final path does not exist: %v", err)
	}
}

// TestIntegration_Move_NoYearInFolderName verifies the Year suffix is omitted
// when the Book has no publication year.
func TestIntegration_Move_NoYearInFolderName(t *testing.T) {
	wd := watchDir(t)
	ld := libraryDir(t)

	torrentName := "no-year-torrent"
	if err := os.WriteFile(filepath.Join(wd, torrentName), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &metadata.Book{Title: "Unknown Year Book", Author: "Some Author"} // Year == 0
	m := library.New(wd, ld, 30*time.Second)

	finalPath, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	t.Logf("moved to: %s", finalPath)

	if strings.Contains(finalPath, "(0)") {
		t.Errorf("finalPath=%q; should not contain '(0)' for zero year", finalPath)
	}
	if _, err := os.Stat(finalPath); err != nil {
		t.Errorf("final path does not exist: %v", err)
	}
}
