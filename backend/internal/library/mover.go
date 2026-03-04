// Package library handles moving completed torrent files from a watch directory
// to the audiobook library, applying Author/Title folder naming.
package library

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"shelfarr/internal/metadata"
)

const defaultPollInterval = 60 * time.Second

// Mover moves completed torrent directories from a watch directory to the
// library directory. It is Syncthing-aware: it polls until the source path
// exists and contains no in-progress Syncthing temp files before moving.
type Mover struct {
	watchDir     string
	libraryDir   string
	pollInterval time.Duration
	pollTimeout  time.Duration
}

// New creates a Mover. watchDir is where Syncthing (or qBit directly) delivers
// completed files; libraryDir is the final audiobook library destination.
// timeout is how long Move will wait for a file to appear before giving up.
func New(watchDir, libraryDir string, timeout time.Duration) *Mover {
	return &Mover{
		watchDir:     watchDir,
		libraryDir:   libraryDir,
		pollInterval: defaultPollInterval,
		pollTimeout:  timeout,
	}
}

// Move waits for torrentName to appear in the watch directory (with no active
// Syncthing temp files), then links all files it contains (flattened, no
// subdirectory nesting) into libraryDir/<Author>/<Title>/.
// Returns the absolute path of that destination directory.
func (m *Mover) Move(ctx context.Context, torrentName string, book *metadata.Book) (string, error) {
	src := filepath.Join(m.watchDir, torrentName)
	slog.Info("library: waiting for file in watch dir", "path", src)

	if err := m.waitForFile(ctx, src); err != nil {
		return "", fmt.Errorf("library: wait for file: %w", err)
	}

	destDir := filepath.Join(m.libraryDir, destSubpath(book))
	if err := os.MkdirAll(destDir, 0o750); err != nil {
		return "", fmt.Errorf("library: create dest dir: %w", err)
	}

	slog.Info("library: linking files", "src", src, "dest", destDir)

	if err := linkFlat(src, destDir); err != nil {
		return "", fmt.Errorf("library: link: %w", err)
	}

	slog.Info("library: link complete", "dest", destDir)
	return destDir, nil
}

// waitForFile polls path every pollInterval until:
//   - path exists and contains no Syncthing in-progress temp files, OR
//   - ctx is cancelled, OR
//   - pollTimeout elapses.
func (m *Mover) waitForFile(ctx context.Context, path string) error {
	deadline := time.Now().Add(m.pollTimeout)
	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	// Check immediately before the first tick.
	if ready, _ := isReady(path); ready {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case t := <-ticker.C:
			if t.After(deadline) {
				return fmt.Errorf("timed out after %s waiting for %q", m.pollTimeout, path)
			}
			ready, err := isReady(path)
			if err != nil {
				slog.Warn("library: poll error", "path", path, "err", err)
				continue
			}
			if ready {
				return nil
			}
			slog.Info("library: file not yet available in watch dir, retrying", "path", path)
		}
	}
}

// isReady returns true when path exists and contains no Syncthing temp files
// (files matching the pattern .syncthing.*.tmp), indicating a complete sync.
func isReady(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		// Single-file torrent — file existence means the sync is complete.
		return true, nil
	}

	// For directories, walk to check for any Syncthing in-progress temp files.
	hasTmp := false
	walkErr := filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		name := d.Name()
		if strings.HasPrefix(name, ".syncthing.") && strings.HasSuffix(name, ".tmp") {
			hasTmp = true
			return fs.SkipAll
		}
		return nil
	})
	if walkErr != nil {
		return false, walkErr
	}
	return !hasTmp, nil
}

// ── folder naming ─────────────────────────────────────────────────────────────

// destSubpath returns the relative Author/Title path inside libraryDir.
func destSubpath(book *metadata.Book) string {
	return filepath.Join(sanitizeName(book.Author), sanitizeName(book.Title))
}

// invalidChars matches characters that are invalid or unsafe in Linux path
// components: forward-slash, backslash, colon, asterisk, question mark,
// double-quote, angle brackets, pipe, and ASCII control characters.
var invalidChars = regexp.MustCompile(`[/\\:*?"<>|\x00-\x1f]`)

// sanitizeName strips path-invalid characters from s and normalises whitespace.
// Returns "Unknown" if the result would be empty.
func sanitizeName(s string) string {
	s = invalidChars.ReplaceAllString(s, "")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)
	if s == "" {
		return "Unknown"
	}
	return s
}

// ── file operations ───────────────────────────────────────────────────────────

// linkFlat links every regular file under src directly into dst, stripping all
// subdirectory nesting. dst must already exist. For each file it tries
// os.Link first (instant, same-device hard link) and falls back to copyFile.
// Duplicate filenames (same base name from different subdirs) are skipped with
// a warning rather than overwriting.
func linkFlat(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		// Single-file torrent: link directly into dst.
		return linkOneFile(src, filepath.Join(dst, filepath.Base(src)), info.Mode())
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		target := filepath.Join(dst, d.Name())
		if _, statErr := os.Stat(target); statErr == nil {
			// Filename collision: prefix with the relative parent directory to
			// avoid dropping files (common in multi-disc torrents where each
			// disc reuses track numbers like "01.mp3").
			rel, relErr := filepath.Rel(src, filepath.Dir(path))
			if relErr != nil || rel == "." {
				slog.Warn("library: skipping top-level duplicate filename", "file", d.Name(), "src", path)
				return nil
			}
			prefix := sanitizeName(strings.ReplaceAll(rel, string(filepath.Separator), " - "))
			target = filepath.Join(dst, prefix+" - "+d.Name())
			if _, statErr2 := os.Stat(target); statErr2 == nil {
				slog.Warn("library: skipping duplicate filename even with prefix", "file", d.Name(), "src", path)
				return nil
			}
			slog.Info("library: renamed duplicate to avoid collision", "new_name", filepath.Base(target), "src", path)
		}
		return linkOneFile(path, target, fi.Mode())
	})
}

// linkOneFile hard-links src to dst, falling back to copyFile on failure
// (e.g. cross-device).
func linkOneFile(src, dst string, mode os.FileMode) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst, mode)
}

// copyAll recursively copies src to dst, preserving file modes.
func copyAll(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDir(src, dst, info.Mode())
	}
	return copyFile(src, dst, info.Mode())
}

func copyDir(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(dst, mode); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyAll(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) //nolint:gosec
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		if cerr := out.Close(); cerr != nil {
			slog.Warn("library: close dest file after copy error", "dst", dst, "err", cerr)
		}
		return err
	}
	return out.Close()
}
