package library

import (
	"os"
	"path/filepath"
	"testing"

	"shelfarr/internal/metadata"
)

func TestListAudioFiles_Basic(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"ch1.mp3", "ch2.m4b", "cover.jpg", "metadata.opf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	audio := listAudioFiles(dir)
	if len(audio) != 2 {
		t.Errorf("expected 2 audio files, got %d: %v", len(audio), audio)
	}
}

func TestListAudioFiles_NonExistent(t *testing.T) {
	audio := listAudioFiles(filepath.Join(t.TempDir(), "no-such-dir"))
	if audio != nil {
		t.Errorf("expected nil for non-existent dir, got %v", audio)
	}
}

func TestSafeRename_SourceNotFound(t *testing.T) {
	dir := t.TempDir()
	err := safeRename(filepath.Join(dir, "nope"), filepath.Join(dir, "dest"))
	if err == nil {
		t.Error("expected error when source does not exist")
	}
}

func TestSafeRename_Success(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := safeRename(src, dst); err != nil {
		t.Fatalf("safeRename: %v", err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Error("destination should exist after rename")
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Error("source should not exist after rename")
	}
}

func TestCleanupAll_NonExistentDir(t *testing.T) {
	_, _, errs := CleanupAll(filepath.Join(t.TempDir(), "no-such"))
	if len(errs) == 0 {
		t.Error("expected error for non-existent directory")
	}
}

func TestCleanupAll_NothingToClean(t *testing.T) {
	libDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(libDir, "Brandon Sanderson", "The Final Empire"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, cleaned, errs := CleanupAll(libDir)
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned, got %d", cleaned)
	}
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
}

func TestCleanupAll_RenamesBook(t *testing.T) {
	libDir := t.TempDir()
	// Double space is valid on all OSes but sanitizeName collapses it to one space.
	dirty := filepath.Join(libDir, "Brandon Sanderson", "Mistborn  The Final Empire")
	if err := os.MkdirAll(dirty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirty, "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, cleaned, errs := CleanupAll(libDir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}
	if _, err := os.Stat(dirty); !os.IsNotExist(err) {
		t.Error("old folder should not exist after cleanup")
	}
	clean := filepath.Join(libDir, "Brandon Sanderson", "Mistborn The Final Empire")
	if _, err := os.Stat(clean); err != nil {
		t.Errorf("renamed folder should exist at %q: %v", clean, err)
	}
}

func TestCleanupBook_NoOpWhenNamesMatch(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Brandon Sanderson", "The Final Empire")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := BookEntry{
		AuthorFolder: "Brandon Sanderson",
		TitleFolder:  "The Final Empire",
		Path:         titlePath,
		Metadata:     &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson"},
	}
	if err := CleanupBook(libDir, entry); err != nil {
		t.Fatalf("CleanupBook: %v", err)
	}
	if _, err := os.Stat(titlePath); err != nil {
		t.Errorf("folder should still exist unchanged: %v", err)
	}
}

func TestCleanupBook_RenamesTitleFolder(t *testing.T) {
	libDir := t.TempDir()
	authorPath := filepath.Join(libDir, "Brandon Sanderson")
	// Double space: valid on all OSes, collapsed to single space by sanitizeName.
	titlePath := filepath.Join(authorPath, "Mistborn  The Final Empire")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := BookEntry{
		AuthorFolder: "Brandon Sanderson",
		TitleFolder:  "Mistborn  The Final Empire",
		Path:         titlePath,
		Metadata:     &metadata.Book{Title: "Mistborn The Final Empire", Author: "Brandon Sanderson"},
	}
	if err := CleanupBook(libDir, entry); err != nil {
		t.Fatalf("CleanupBook: %v", err)
	}
	if _, err := os.Stat(filepath.Join(authorPath, "Mistborn The Final Empire")); err != nil {
		t.Errorf("renamed title folder should exist: %v", err)
	}
	if _, err := os.Stat(titlePath); !os.IsNotExist(err) {
		t.Error("old title folder should not exist after rename")
	}
}

func TestCleanupBook_RenamesAuthorFolder(t *testing.T) {
	libDir := t.TempDir()
	// Double space in author name: valid on all OSes, collapsed by sanitizeName.
	authorPath := filepath.Join(libDir, "Brandon  Sanderson")
	titlePath := filepath.Join(authorPath, "The Final Empire")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := BookEntry{
		AuthorFolder: "Brandon  Sanderson",
		TitleFolder:  "The Final Empire",
		Path:         titlePath,
		Metadata:     &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson"},
	}
	if err := CleanupBook(libDir, entry); err != nil {
		t.Fatalf("CleanupBook: %v", err)
	}
	clean := filepath.Join(libDir, "Brandon Sanderson", "The Final Empire")
	if _, err := os.Stat(clean); err != nil {
		t.Errorf("renamed author+title folder should exist: %v", err)
	}
}

func TestCleanupBook_MergesIntoExistingAuthorFolder(t *testing.T) {
	libDir := t.TempDir()
	// Target author folder already exists with another book.
	if err := os.MkdirAll(filepath.Join(libDir, "Brandon Sanderson", "Elantris"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Source book under an author folder with double space.
	authorPath := filepath.Join(libDir, "Brandon  Sanderson")
	titlePath := filepath.Join(authorPath, "The Final Empire")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := BookEntry{
		AuthorFolder: "Brandon  Sanderson",
		TitleFolder:  "The Final Empire",
		Path:         titlePath,
		Metadata:     &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson"},
	}
	if err := CleanupBook(libDir, entry); err != nil {
		t.Fatalf("CleanupBook merge: %v", err)
	}
	// Title should now be inside the existing author folder.
	merged := filepath.Join(libDir, "Brandon Sanderson", "The Final Empire")
	if _, err := os.Stat(merged); err != nil {
		t.Errorf("merged title folder should exist at %q: %v", merged, err)
	}
}

func TestCleanupBook_RenamesSingleAudioFile(t *testing.T) {
	libDir := t.TempDir()
	authorPath := filepath.Join(libDir, "Frank Herbert")
	titlePath := filepath.Join(authorPath, "Dune")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	// Single audio file with a mismatched name.
	if err := os.WriteFile(filepath.Join(titlePath, "wrong-name.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := BookEntry{
		AuthorFolder: "Frank Herbert",
		TitleFolder:  "Dune",
		Path:         titlePath,
		Metadata:     &metadata.Book{Title: "Dune", Author: "Frank Herbert"},
	}
	if err := CleanupBook(libDir, entry); err != nil {
		t.Fatalf("CleanupBook: %v", err)
	}
	// Audio file should be renamed to "Dune.mp3".
	if _, err := os.Stat(filepath.Join(titlePath, "Dune.mp3")); err != nil {
		t.Errorf("renamed audio file Dune.mp3 should exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(titlePath, "wrong-name.mp3")); !os.IsNotExist(err) {
		t.Error("old audio filename should not exist after rename")
	}
}

func TestCleanupAll_EncodeOnlyIncludedButNotCounted(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Frank Herbert", "Dune")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	// Single MP3 with clean name — NeedsEncode=true, NeedsRename=false, NeedsFlat=false.
	if err := os.WriteFile(filepath.Join(titlePath, "Dune.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, cleaned, errs := CleanupAll(libDir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cleaned != 0 {
		t.Errorf("Cleaned=%d; want 0 (no rename/flatten needed)", cleaned)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 actionable entry for encode-only book, got %d", len(entries))
	}
	if len(entries) > 0 && !entries[0].NeedsEncode {
		t.Error("returned entry should have NeedsEncode=true")
	}
}

func TestCleanupAll_SingleM4BSkipped(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Frank Herbert", "Dune")
	if err := os.MkdirAll(titlePath, 0o755); err != nil {
		t.Fatal(err)
	}
	// Already a single M4B — nothing to do.
	if err := os.WriteFile(filepath.Join(titlePath, "Dune.m4b"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, cleaned, errs := CleanupAll(libDir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cleaned != 0 {
		t.Errorf("Cleaned=%d; want 0", cleaned)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 actionable entries for single-M4B book, got %d", len(entries))
	}
}

func TestFlattenBookDir_FlattensSubdirs(t *testing.T) {
	dir := t.TempDir()
	titleDir := filepath.Join(dir, "The Book")
	if err := os.MkdirAll(filepath.Join(titleDir, "disc1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(titleDir, "disc2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titleDir, "disc1", "01.mp3"), []byte("d1ch1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titleDir, "disc2", "01.mp3"), []byte("d2ch1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titleDir, "disc1", "02.mp3"), []byte("d1ch2"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := flattenBookDir(titleDir); err != nil {
		t.Fatalf("flattenBookDir: %v", err)
	}

	// Subdirs must be gone.
	if hasNestedDirs(titleDir) {
		t.Error("title dir should have no subdirs after flatten")
	}
	// 01.mp3 appears in both discs → both prefixed; 02.mp3 is unique → no prefix.
	for _, want := range []string{"disc1 - 01.mp3", "disc2 - 01.mp3", "02.mp3"} {
		if _, err := os.Stat(filepath.Join(titleDir, want)); err != nil {
			t.Errorf("expected file %q: %v", want, err)
		}
	}
}

func TestCleanupBook_FlattensNestedDirs(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Frank Herbert", "Dune")
	if err := os.MkdirAll(filepath.Join(titlePath, "part1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(titlePath, "part1", "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := BookEntry{
		AuthorFolder: "Frank Herbert",
		TitleFolder:  "Dune",
		Path:         titlePath,
		NeedsFlat:    true,
		Metadata:     &metadata.Book{Title: "Dune", Author: "Frank Herbert"},
	}
	if err := CleanupBook(libDir, entry); err != nil {
		t.Fatalf("CleanupBook: %v", err)
	}
	if hasNestedDirs(titlePath) {
		t.Error("title dir should be flat after cleanup")
	}
	// Single audio file gets renamed to the title name by CleanupBook step 3.
	if _, err := os.Stat(filepath.Join(titlePath, "Dune.mp3")); err != nil {
		t.Errorf("flattened+renamed audio file Dune.mp3 should exist: %v", err)
	}
}

func TestCleanupAll_FlattensNestedDirs(t *testing.T) {
	libDir := t.TempDir()
	titlePath := filepath.Join(libDir, "Frank Herbert", "Dune")
	if err := os.MkdirAll(filepath.Join(titlePath, "part1"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Use two audio files so single-file rename doesn't apply — keeps the test
	// focused on flattening rather than renaming.
	for _, name := range []string{"ch1.mp3", "ch2.mp3"} {
		if err := os.WriteFile(filepath.Join(titlePath, "part1", name), []byte("audio"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, cleaned, errs := CleanupAll(libDir)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}
	if hasNestedDirs(titlePath) {
		t.Error("title dir should be flat after CleanupAll")
	}
}
