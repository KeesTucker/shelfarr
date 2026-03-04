package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"shelfarr/internal/metadata"
)

// ── sanitizeName ──────────────────────────────────────────────────────────────

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"Brandon Sanderson", "Brandon Sanderson"},
		{"  extra  spaces  ", "extra spaces"},
		{"Multiple   internal   gaps", "Multiple internal gaps"},
		{"Invalid/Chars", "InvalidChars"},
		{"Colon: in title", "Colon in title"},
		{`Back\slash`, "Backslash"},
		{`Ast*risk`, "Astrisk"},
		{`Quest?ion`, "Question"},
		{`Quot"es`, "Quotes"},
		{"Less<Than", "LessThan"},
		{"Great>er", "Greater"},
		{"Pi|pe", "Pipe"},
		{"Control\x00Char", "ControlChar"},
		{"Tab\x09Here", "TabHere"},
		{"", "Unknown"},
		{"   ", "Unknown"},
		{"///", "Unknown"}, // all invalid → stripped → empty → "Unknown"
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got := sanitizeName(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeName(%q) = %q; want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ── destSubpath ───────────────────────────────────────────────────────────────

func TestDestSubpath(t *testing.T) {
	tests := []struct {
		name string
		book *metadata.Book
		want string
	}{
		{
			name: "full metadata",
			book: &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson", Year: 2006},
			want: filepath.Join("Brandon Sanderson", "The Final Empire"),
		},
		{
			name: "no year",
			book: &metadata.Book{Title: "Dune", Author: "Frank Herbert"},
			want: filepath.Join("Frank Herbert", "Dune"),
		},
		{
			name: "invalid chars in title and author are stripped",
			book: &metadata.Book{Title: "Title: With Colon", Author: "Author/Slash", Year: 2000},
			want: filepath.Join("AuthorSlash", "Title With Colon"),
		},
		{
			name: "empty author falls back to Unknown",
			book: &metadata.Book{Title: "Some Book", Author: "", Year: 1999},
			want: filepath.Join("Unknown", "Some Book"),
		},
		{
			name: "empty title falls back to Unknown",
			book: &metadata.Book{Title: "", Author: "Known Author"},
			want: filepath.Join("Known Author", "Unknown"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := destSubpath(tc.book)
			if got != tc.want {
				t.Errorf("destSubpath = %q; want %q", got, tc.want)
			}
		})
	}
}

// ── isReady ───────────────────────────────────────────────────────────────────

func TestIsReady_NonExistent(t *testing.T) {
	ready, err := isReady(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not ready for non-existent path")
	}
}

func TestIsReady_SingleFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "audiobook.mp3")
	if err := os.WriteFile(f, []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	ready, err := isReady(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("expected ready for existing single file")
	}
}

func TestIsReady_DirectoryNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	ready, err := isReady(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("expected ready for directory with no Syncthing temp files")
	}
}

func TestIsReady_DirectoryWithSyncthingTempFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Syncthing in-progress temp file.
	if err := os.WriteFile(filepath.Join(dir, ".syncthing.ch1.mp3.tmp"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	ready, err := isReady(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not ready when Syncthing temp file is present at top level")
	}
}

func TestIsReady_NestedSyncthingTempFile(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "disc1")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Temp file nested in a subdirectory.
	if err := os.WriteFile(filepath.Join(sub, ".syncthing.track.mp3.tmp"), []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}
	ready, err := isReady(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ready {
		t.Error("expected not ready when nested Syncthing temp file is present")
	}
}

// A file named similarly but not matching the .syncthing.*.tmp pattern must
// not block readiness.
func TestIsReady_NonSyncthingDotFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".syncthing_done"), []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}
	ready, err := isReady(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ready {
		t.Error("expected ready: neither file matches the .syncthing.*.tmp pattern")
	}
}

// ── linkFlat ──────────────────────────────────────────────────────────────────

func TestLinkFlat_NestedDirs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "book")
	dst := filepath.Join(dir, "dest")
	if err := os.MkdirAll(filepath.Join(src, "disc1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "disc2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"disc1/ch01.mp3": "audio1",
		"disc1/ch02.mp3": "audio2",
		"disc2/ch03.mp3": "audio3",
		"cover.jpg":      "image",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(src, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := linkFlat(src, dst); err != nil {
		t.Fatalf("linkFlat: %v", err)
	}

	// All files should be directly in dst with no subdirectories.
	want := []string{"ch01.mp3", "ch02.mp3", "ch03.mp3", "cover.jpg"}
	for _, name := range want {
		if _, err := os.Stat(filepath.Join(dst, name)); err != nil {
			t.Errorf("expected %q directly in dest: %v", name, err)
		}
	}
	// No subdirs should exist in dst.
	entries, _ := os.ReadDir(dst)
	for _, e := range entries {
		if e.IsDir() {
			t.Errorf("unexpected subdirectory %q in dest", e.Name())
		}
	}
}

func TestLinkFlat_DuplicateNamePrefixed(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "book")
	dst := filepath.Join(dir, "dest")
	if err := os.MkdirAll(filepath.Join(src, "disc1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "disc2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	// Same filename in two different subdirs — simulates multi-disc torrents
	// that reuse track numbers (e.g. disc1/01.mp3, disc2/01.mp3).
	if err := os.WriteFile(filepath.Join(src, "disc1", "cover.jpg"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "disc2", "cover.jpg"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := linkFlat(src, dst); err != nil {
		t.Fatalf("linkFlat with duplicate: %v", err)
	}

	// First occurrence lands under its original name.
	got1, err := os.ReadFile(filepath.Join(dst, "cover.jpg"))
	if err != nil {
		t.Fatalf("cover.jpg missing: %v", err)
	}
	if string(got1) != "first" {
		t.Errorf("cover.jpg content=%q; want %q", got1, "first")
	}

	// Second occurrence must be renamed with the parent directory prefix.
	got2, err := os.ReadFile(filepath.Join(dst, "disc2 - cover.jpg"))
	if err != nil {
		t.Fatalf("disc2 - cover.jpg missing: %v", err)
	}
	if string(got2) != "second" {
		t.Errorf("disc2 - cover.jpg content=%q; want %q", got2, "second")
	}
}

// ── copyAll ───────────────────────────────────────────────────────────────────

func TestCopyAll_File(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mp3")
	dst := filepath.Join(dir, "dst.mp3")
	if err := os.WriteFile(src, []byte("audio content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyAll(src, dst); err != nil {
		t.Fatalf("copyAll (file): %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "audio content" {
		t.Errorf("dst content=%q; want %q", got, "audio content")
	}
	// Source must still exist after copy (copyAll does not delete).
	if _, err := os.Stat(src); err != nil {
		t.Error("source should still exist after copyAll")
	}
}

func TestCopyAll_Directory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"root.mp3":       "root audio",
		"sub/nested.mp3": "nested audio",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(src, rel), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := copyAll(src, dst); err != nil {
		t.Fatalf("copyAll (dir): %v", err)
	}

	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Errorf("missing %q at destination: %v", rel, err)
			continue
		}
		if string(got) != want {
			t.Errorf("%q content=%q; want %q", rel, got, want)
		}
	}
}

// ── Move ──────────────────────────────────────────────────────────────────────

// newFastMover creates a Mover with a short poll interval for tests.
func newFastMover(watchDir, libraryDir string, timeout time.Duration) *Mover {
	m := New(watchDir, libraryDir, timeout)
	m.pollInterval = 25 * time.Millisecond
	return m
}

func TestMove_HappyPath(t *testing.T) {
	watchDir := t.TempDir()
	libDir := t.TempDir()

	torrentName := "Brandon Sanderson - Mistborn"
	src := filepath.Join(watchDir, torrentName)
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "ch1.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson", Year: 2006}
	m := newFastMover(watchDir, libDir, 5*time.Second)

	finalPath, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move: %v", err)
	}

	// Destination is the Author/Title dir; files are flattened into it.
	wantPath := filepath.Join(libDir, "Brandon Sanderson", "The Final Empire")
	if finalPath != wantPath {
		t.Errorf("finalPath=%q; want %q", finalPath, wantPath)
	}
	// File should be directly in destDir, not inside a sub-folder.
	if _, err := os.Stat(filepath.Join(finalPath, "ch1.mp3")); err != nil {
		t.Errorf("ch1.mp3 not at destination: %v", err)
	}
	// Source must still exist (link, not move).
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source should still exist after link: %v", err)
	}
}

func TestMove_WaitsForFileToAppear(t *testing.T) {
	watchDir := t.TempDir()
	libDir := t.TempDir()

	torrentName := "Dune - Frank Herbert"
	book := &metadata.Book{Title: "Dune", Author: "Frank Herbert", Year: 1965}
	m := newFastMover(watchDir, libDir, 10*time.Second)

	// Deliver the file after a few poll intervals.
	go func() {
		time.Sleep(120 * time.Millisecond)
		src := filepath.Join(watchDir, torrentName)
		_ = os.Mkdir(src, 0o755)
		_ = os.WriteFile(filepath.Join(src, "dune.mp3"), []byte("audio"), 0o644)
	}()

	finalPath, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move with delayed delivery: %v", err)
	}
	if _, err := os.Stat(filepath.Join(finalPath, "dune.mp3")); err != nil {
		t.Errorf("dune.mp3 not at destination: %v", err)
	}
}

func TestMove_WaitsForSyncthingTempToDisappear(t *testing.T) {
	watchDir := t.TempDir()
	libDir := t.TempDir()

	torrentName := "Mistborn"
	src := filepath.Join(watchDir, torrentName)
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "mistborn.mp3"), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmpFile := filepath.Join(src, ".syncthing.mistborn.mp3.tmp")
	if err := os.WriteFile(tmpFile, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &metadata.Book{Title: "The Final Empire", Author: "Brandon Sanderson", Year: 2006}
	m := newFastMover(watchDir, libDir, 10*time.Second)

	// Remove the Syncthing temp file after a few poll intervals.
	go func() {
		time.Sleep(120 * time.Millisecond)
		_ = os.Remove(tmpFile)
	}()

	_, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move with Syncthing temp file: %v", err)
	}
}

func TestMove_TimeoutIfFileNeverAppears(t *testing.T) {
	watchDir := t.TempDir()
	libDir := t.TempDir()

	book := &metadata.Book{Title: "Ghost", Author: "Nobody"}
	m := newFastMover(watchDir, libDir, 60*time.Millisecond)

	_, err := m.Move(context.Background(), "ghost-torrent", book)
	if err == nil {
		t.Fatal("expected timeout error when file never appears")
	}
}

func TestMove_ContextCancelled(t *testing.T) {
	watchDir := t.TempDir()
	libDir := t.TempDir()

	book := &metadata.Book{Title: "Cancelled", Author: "Author"}
	m := newFastMover(watchDir, libDir, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Move is called

	_, err := m.Move(ctx, "never-exists", book)
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

func TestMove_SingleFileTorrent(t *testing.T) {
	watchDir := t.TempDir()
	libDir := t.TempDir()

	// Single-file torrent: the torrent "name" is the file itself, not a directory.
	torrentName := "dune-frank-herbert.mp3"
	if err := os.WriteFile(filepath.Join(watchDir, torrentName), []byte("audio"), 0o644); err != nil {
		t.Fatal(err)
	}

	book := &metadata.Book{Title: "Dune", Author: "Frank Herbert", Year: 1965}
	m := newFastMover(watchDir, libDir, 5*time.Second)

	finalPath, err := m.Move(context.Background(), torrentName, book)
	if err != nil {
		t.Fatalf("Move single file: %v", err)
	}

	wantPath := filepath.Join(libDir, "Frank Herbert", "Dune")
	if finalPath != wantPath {
		t.Errorf("finalPath=%q; want %q", finalPath, wantPath)
	}
	if _, err := os.Stat(filepath.Join(finalPath, torrentName)); err != nil {
		t.Errorf("file not at destination: %v", err)
	}
}
