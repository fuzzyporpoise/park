// Package store manages the on-disk category folders and the notes inside
// them: scanning, resolving paths, and reclassifying.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/note"
)

// Item is a single parked note as seen by the scanner/TUI.
type Item struct {
	note.Metadata
	Path     string
	Filename string
	ModTime  time.Time
}

// Init creates the category folders defined in cfg. It returns the paths
// that were created and the paths that already existed. It is safe to run
// repeatedly: folders that already exist are left untouched.
func Init(cfg *config.Config) (created []string, existed []string, err error) {
	for _, cl := range cfg.Categories {
		info, statErr := os.Stat(cl.Path)
		if statErr == nil && info.IsDir() {
			existed = append(existed, cl.Path)
			continue
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, nil, fmt.Errorf("stat category folder %q: %w", cl.Path, statErr)
		}
		if mkdirErr := os.MkdirAll(cl.Path, 0o755); mkdirErr != nil {
			return nil, nil, fmt.Errorf("create category folder %q: %w", cl.Path, mkdirErr)
		}
		created = append(created, cl.Path)
	}
	return created, existed, nil
}

// Check returns the list of category paths that do not exist as directories.
// It returns an error if a path cannot be inspected for reasons other than
// not existing, or if the path exists but is not a directory.
func Check(cfg *config.Config) ([]string, error) {
	var missing []string
	for _, cl := range cfg.Categories {
		info, err := os.Stat(cl.Path)
		if err == nil {
			if !info.IsDir() {
				return nil, fmt.Errorf("category path %q exists but is not a directory", cl.Path)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat category folder %q: %w", cl.Path, err)
		}
		missing = append(missing, cl.Path)
	}
	return missing, nil
}

// Scan lists all markdown files under a given category folder, sorted oldest
// first (so the longest-neglected items surface at the top).
func Scan(cfg *config.Config, categoryName string) ([]Item, error) {
	cl, ok := cfg.CategoryByName(categoryName)
	if !ok {
		return nil, cfg.UnknownCategoryError(categoryName)
	}

	entries, err := os.ReadDir(cl.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read category folder %q: %w", cl.Path, err)
	}

	var items []Item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(cl.Path, e.Name())
		n, err := note.Parse(path)
		if err != nil {
			return nil, fmt.Errorf("parse frontmatter for %q: %w", path, err)
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", path, err)
		}
		items = append(items, Item{
			Metadata: n.Metadata,
			Path:     path,
			Filename: e.Name(),
			ModTime:  info.ModTime(),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ModTime.Before(items[j].ModTime)
	})
	return items, nil
}

// Reclassify moves a file into the target category folder and rewrites its
// frontmatter category field to match. The file may be named by a bare
// basename (searched across all category folders) or by a literal path, using
// the same resolution rules as ResolvePath. This is the "triage decision"
// primitive everything else builds on.
func Reclassify(cfg *config.Config, filename string, targetCategory string) error {
	cl, ok := cfg.CategoryByName(targetCategory)
	if !ok {
		return cfg.UnknownCategoryError(targetCategory)
	}

	src, err := ResolvePath(cfg, filename)
	if err != nil {
		return err
	}
	src, err = filepath.Abs(src)
	if err != nil {
		return fmt.Errorf("resolve path %q: %w", filename, err)
	}

	if filepath.Dir(src) == cl.Path {
		return fmt.Errorf("already in %s", targetCategory)
	}

	n, err := note.Parse(src)
	if err != nil {
		return fmt.Errorf("parse frontmatter for %q: %w", src, err)
	}

	n.Category = targetCategory
	base := filepath.Base(src)
	dst := filepath.Join(cl.Path, base)

	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("already exists in %s: %s", targetCategory, base)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check destination %q: %w", dst, err)
	}

	// Rewrite frontmatter in place first, then move; if the move fails
	// (e.g. cross-device), the file is still left in a consistent state.
	if err := note.Write(src, n); err != nil {
		return fmt.Errorf("rewrite frontmatter for %q: %w", src, err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("move %q to %q: %w", src, dst, err)
	}
	return nil
}

// FormatInitResult formats the result of Init for user-facing output.
func FormatInitResult(created, existed []string) string {
	if len(created) == 0 {
		return "all park folders already exist"
	}

	msg := fmt.Sprintf("created park folders: %s", strings.Join(created, ", "))
	if len(existed) > 0 {
		msg += fmt.Sprintf(" (%s already existed)", strings.Join(existed, ", "))
	}
	return msg
}

// ResolvePath accepts either a bare basename (searched across all category
// folders) or a path used as-is. A basename that contains a path separator is
// treated as a literal path and is never joined onto a category folder, so a
// path can never double-join. It returns os.ErrNotExist when no file can be
// resolved.
func ResolvePath(cfg *config.Config, filename string) (string, error) {
	if _, err := os.Stat(filename); err == nil {
		return filename, nil
	}
	if strings.ContainsAny(filename, `/\`) {
		return "", os.ErrNotExist
	}
	for _, cl := range cfg.Categories {
		p := filepath.Join(cl.Path, filename)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", os.ErrNotExist
}
