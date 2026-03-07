package library

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"shelfarr/internal/metadata"
)

// flattenBookDir moves all files from subdirectories directly into dir using
// the same duplicate-prefixing rules as linkFlat, then removes the original
// tree. It operates atomically: files are linked into a sibling temp directory
// first, then the original is swapped out only on success.
func flattenBookDir(dir string) error {
	tmpDir, err := os.MkdirTemp(filepath.Dir(dir), ".flatten-*") //nolint:gosec
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	if err := linkFlat(dir, tmpDir); err != nil {
		_ = os.RemoveAll(tmpDir) //nolint:gosec
		return fmt.Errorf("flatten files: %w", err)
	}
	backup := dir + ".bak"
	if err := os.Rename(dir, backup); err != nil { //nolint:gosec
		_ = os.RemoveAll(tmpDir) //nolint:gosec
		return fmt.Errorf("backup title dir: %w", err)
	}
	if err := os.Rename(tmpDir, dir); err != nil { //nolint:gosec
		_ = os.Rename(backup, dir) //nolint:gosec // best-effort restore
		return fmt.Errorf("install flattened dir: %w", err)
	}
	return os.RemoveAll(backup) //nolint:gosec
}

// CleanupBook renames the book folder, audio files, and patches the OPF to
// match the naming rules (sanitizeName(Author)/sanitizeName(Title)/).
//
// Steps:
//  1. Re-resolve metadata from OPF / ABS json / folder name.
//  2. If nested subdirectories are present: flatten all files to the top level.
//  3. If single audio file: rename to sanitizeName(title)+ext.
//  4. Patch OPF (try edit, fall back to regen).
//  5. Rename title folder if needed.
//  6. Rename author folder if needed (merges into existing if target exists).
func CleanupBook(libraryDir string, entry BookEntry) error {
	dir := entry.Path

	// 1. Re-resolve metadata fresh from disk (in case it changed since scan).
	files := collectFiles(dir)
	book, _ := resolveMetadata(dir, entry.TitleFolder, entry.AuthorFolder, files)

	// 2. Flatten nested subdirectories if present.
	if hasNestedDirs(dir) {
		if err := flattenBookDir(dir); err != nil {
			return fmt.Errorf("flatten book dir: %w", err)
		}
		files = collectFiles(dir)
	}

	_ = files // consumed by resolveMetadata; re-collected after flatten if needed

	expectedTitle := sanitizeName(book.Title)
	expectedAuthor := sanitizeName(book.Author)

	// 3. Rename single audio file.
	audioFiles := listAudioFiles(dir)
	if len(audioFiles) == 1 {
		old := audioFiles[0]
		ext := strings.ToLower(filepath.Ext(old))
		newName := expectedTitle + ext
		if filepath.Base(old) != newName {
			if err := os.Rename(old, filepath.Join(dir, newName)); err != nil { //nolint:gosec
				slog.Warn("library cleanup: rename audio file", "old", old, "new", newName, "err", err) //nolint:gosec
				// non-fatal — continue with folder renames
			}
		}
	}

	// 4. Patch OPF (or regen on failure).
	opfPaths, _ := filepath.Glob(filepath.Join(dir, "*.opf"))
	if len(opfPaths) > 0 {
		best := metadata.ChooseBestOPF(opfPaths, entry.TitleFolder)
		if err := metadata.PatchOPF(best, book); err != nil {
			slog.Warn("library cleanup: patch OPF failed, regenerating", "path", best, "err", err) //nolint:gosec
			if err := metadata.EnsureOPF(dir, book); err != nil {
				slog.Warn("library cleanup: regen OPF", "path", dir, "err", err) //nolint:gosec
			}
		}
	}

	// Track the title folder path as it moves through renames.
	currentTitlePath := dir
	currentAuthorFolder := entry.AuthorFolder

	// 5. Rename title folder.
	if expectedTitle != entry.TitleFolder {
		newTitlePath := filepath.Join(libraryDir, currentAuthorFolder, expectedTitle)
		if err := safeRename(currentTitlePath, newTitlePath); err != nil {
			return fmt.Errorf("rename title folder %q → %q: %w", currentTitlePath, newTitlePath, err)
		}
		currentTitlePath = newTitlePath
	}

	// 6. Rename author folder.
	if expectedAuthor != currentAuthorFolder {
		oldAuthorPath := filepath.Join(libraryDir, currentAuthorFolder)
		newAuthorPath := filepath.Join(libraryDir, expectedAuthor)

		// If the target author folder already exists, move just our title subfolder into it.
		if _, err := os.Stat(newAuthorPath); err == nil { //nolint:gosec
			dest := filepath.Join(newAuthorPath, expectedTitle)
			if err := safeRename(currentTitlePath, dest); err != nil {
				return fmt.Errorf("move title into existing author folder: %w", err)
			}
			// Remove old author folder if now empty.
			_ = os.Remove(oldAuthorPath) //nolint:gosec // silently ignore if not empty
		} else {
			if err := safeRename(oldAuthorPath, newAuthorPath); err != nil {
				return fmt.Errorf("rename author folder %q → %q: %w", oldAuthorPath, newAuthorPath, err)
			}
		}
	}

	return nil
}

// CleanupAll scans libraryDir, finds all books that need renaming, and cleans
// each one. Returns only the successfully-cleaned entries (for callers that
// need to act on them, e.g. to trigger ABS merge), the number cleaned, and
// any errors.
func CleanupAll(libraryDir string) ([]BookEntry, int, []string) {
	entries, err := ScanLibrary(libraryDir)
	if err != nil {
		return nil, 0, []string{fmt.Sprintf("scan library: %s", err)}
	}

	var cleanedEntries []BookEntry
	var errs []string
	for _, e := range entries {
		if !e.NeedsRename && !e.NeedsFlat {
			continue
		}
		if err := CleanupBook(libraryDir, e); err != nil {
			errs = append(errs, fmt.Sprintf("%s/%s: %s", e.AuthorFolder, e.TitleFolder, err))
		} else {
			cleanedEntries = append(cleanedEntries, e)
		}
	}
	return cleanedEntries, len(cleanedEntries), errs
}

// safeRename performs os.Rename but checks that the source exists first.
func safeRename(src, dst string) error {
	if _, err := os.Stat(src); err != nil { //nolint:gosec // paths built from libraryDir+sanitizeName
		return fmt.Errorf("source not found: %w", err)
	}
	return os.Rename(src, dst) //nolint:gosec // paths built from libraryDir+sanitizeName
}

// listAudioFiles returns absolute paths of all audio files directly in dir.
func listAudioFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if audioExts[ext] {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}
