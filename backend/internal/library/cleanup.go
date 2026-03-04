package library

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"shelfarr/internal/metadata"
)

// CleanupBook renames the book folder, audio files, and patches the OPF to
// match the naming rules (sanitizeName(Author)/sanitizeName(Title)/).
//
// Steps:
//  1. Re-resolve metadata from OPF / ABS json / folder name.
//  2. If single audio file: rename to sanitizeName(title)+ext.
//  3. Patch OPF (try edit, fall back to regen).
//  4. Rename title folder if needed.
//  5. Rename author folder if needed (merges into existing if target exists).
func CleanupBook(libraryDir string, entry BookEntry) error {
	dir := entry.Path

	// 1. Re-resolve metadata fresh from disk (in case it changed since scan).
	files := collectFiles(dir)
	book, _ := resolveMetadata(dir, entry.TitleFolder, entry.AuthorFolder, files)

	expectedTitle := sanitizeName(book.Title)
	expectedAuthor := sanitizeName(book.Author)

	// 2. Rename single audio file.
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

	// 3. Patch OPF (or regen on failure).
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

	// 4. Rename title folder.
	if expectedTitle != entry.TitleFolder {
		newTitlePath := filepath.Join(libraryDir, currentAuthorFolder, expectedTitle)
		if err := safeRename(currentTitlePath, newTitlePath); err != nil {
			return fmt.Errorf("rename title folder %q → %q: %w", currentTitlePath, newTitlePath, err)
		}
		currentTitlePath = newTitlePath
	}

	// 5. Rename author folder.
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
// each one. Returns the number cleaned and a slice of error strings.
func CleanupAll(libraryDir string) (int, []string) {
	entries, err := ScanLibrary(libraryDir)
	if err != nil {
		return 0, []string{fmt.Sprintf("scan library: %s", err)}
	}

	cleaned := 0
	var errs []string
	for _, e := range entries {
		if !e.NeedsRename {
			continue
		}
		if err := CleanupBook(libraryDir, e); err != nil {
			errs = append(errs, fmt.Sprintf("%s/%s: %s", e.AuthorFolder, e.TitleFolder, err))
		} else {
			cleaned++
		}
	}
	return cleaned, errs
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
