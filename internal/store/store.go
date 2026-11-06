// Package store manages the on-disk category folders and the notes inside
// them: scanning, resolving paths, and reclassifying.
package store

import (
	"errors"
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
		if e.IsDir() || !strings.HasSuffix(e.Name(), note.Extension) {
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

	if err := rejectOccupied(cfg, src, filepath.Base(src)); err != nil {
		return err
	}

	n, err := note.Parse(src)
	if err != nil {
		return fmt.Errorf("parse frontmatter for %q: %w", src, err)
	}

	n.Category = targetCategory
	base := filepath.Base(src)
	dst := filepath.Join(cl.Path, base)

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

// ErrAmbiguous is returned when a name resolves to notes in more than one
// category folder. Write paths prevent that (see Duplicates), but a store that
// predates the uniqueness invariant can still hold such names.
var ErrAmbiguous = errors.New("note name is ambiguous")

// ResolvePath accepts a bare note name (searched across all category folders) or
// a path used as-is. A name that contains a path separator is treated as a
// literal path and is never joined onto a category folder, so a path can never
// double-join.
//
// A bare name is tried exactly as given first, so an existing name always wins.
// When that misses and the name does not already carry the note extension, the
// same name plus note.Extension is tried, which is what makes `park show kallax`
// resolve kallax.md. Matching more than one category folder is an error wrapping
// ErrAmbiguous rather than a silent pick between copies.
func ResolvePath(cfg *config.Config, filename string) (string, error) {
	if _, err := os.Stat(filename); err == nil {
		return filename, nil
	}
	if strings.ContainsAny(filename, `/\`) {
		return "", notFoundError{fmt.Sprintf("file %q does not exist", filename)}
	}

	match, matches, err := resolveName(cfg, filename)
	if err != nil {
		return "", err
	}
	if match != "" {
		return match, nil
	}
	if len(matches) > 1 {
		return "", ambiguousError(cfg, filename, matches)
	}

	if !strings.HasSuffix(filename, note.Extension) {
		match, matches, err = resolveName(cfg, filename+note.Extension)
		if err != nil {
			return "", err
		}
		if match != "" {
			return match, nil
		}
		if len(matches) > 1 {
			return "", ambiguousError(cfg, filename, matches)
		}
	}

	return "", notFoundError{fmt.Sprintf("note %q not found in any category folder", filename)}
}

// notFoundError carries a message that already explains the miss while still
// matching os.ErrNotExist, so callers keep errors.Is and the CLI does not print
// the redundant "file does not exist" suffix.
type notFoundError struct {
	msg string
}

func (e notFoundError) Error() string { return e.msg }

func (e notFoundError) Unwrap() error { return os.ErrNotExist }

// resolveName looks name up across the category folders. It returns the single
// match when there is exactly one, or every match when there are several so the
// caller can report an ambiguity against the argument the user actually typed.
func resolveName(cfg *config.Config, name string) (string, []string, error) {
	matches, err := note.Locate(cfg, name)
	if err != nil {
		return "", nil, err
	}
	if len(matches) == 1 {
		return matches[0], nil, nil
	}
	return "", matches, nil
}

// ambiguousError names every copy of an ambiguous note so the report is
// actionable without a second command, and points at the command that flags
// such stores.
func ambiguousError(cfg *config.Config, name string, matches []string) error {
	labeled := make([]string, 0, len(matches))
	for _, m := range matches {
		category, ok := note.CategoryForPath(cfg, m)
		if !ok {
			category = "unknown"
		}
		labeled = append(labeled, fmt.Sprintf("%s: %s", category, m))
	}
	return fmt.Errorf("%w: %q matches %d copies (%s); inspect them with `park check`",
		ErrAmbiguous, name, len(matches), strings.Join(labeled, ", "))
}

// rejectOccupied returns an error when a note named filename already exists in
// a category folder other than the one holding src, naming the colliding path.
// Note names are unique across the whole store, so a collision anywhere blocks
// the move even when the destination folder itself is empty.
func rejectOccupied(cfg *config.Config, src, filename string) error {
	matches, err := note.Locate(cfg, filename)
	if err != nil {
		return err
	}
	for _, other := range matches {
		abs, err := filepath.Abs(other)
		if err != nil {
			return fmt.Errorf("resolve path %q: %w", other, err)
		}
		if abs == src {
			continue
		}
		category, ok := note.CategoryForPath(cfg, other)
		if !ok {
			category = "another category"
		}
		return fmt.Errorf("note already exists in %s: %s", category, abs)
	}
	return nil
}

// Duplicate is a note filename that exists in more than one category folder.
type Duplicate struct {
	Filename string
	Copies   []DuplicateCopy
}

// DuplicateCopy is one on-disk copy of a duplicated note name. Category is the
// configured category name, not the folder name, so it matches what `park
// list` prints.
type DuplicateCopy struct {
	Category string
	Path     string
}

// Duplicates reports every note filename present in more than one category
// folder, sorted by filename, with each copy listed in config order. Notes are
// named uniquely across the store, so a round of `park new` writes or an
// external copy can leave duplicates behind that write-time checks never saw;
// this is the read path for finding them.
func Duplicates(cfg *config.Config) ([]Duplicate, error) {
	copies := make(map[string][]DuplicateCopy)
	for _, cl := range cfg.Categories {
		entries, err := os.ReadDir(cl.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read category folder %q: %w", cl.Path, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), note.Extension) {
				continue
			}
			copies[e.Name()] = append(copies[e.Name()], DuplicateCopy{
				Category: cl.Name,
				Path:     filepath.Join(cl.Path, e.Name()),
			})
		}
	}

	var dups []Duplicate
	for name, found := range copies {
		if len(found) > 1 {
			dups = append(dups, Duplicate{Filename: name, Copies: found})
		}
	}
	sort.Slice(dups, func(i, j int) bool { return dups[i].Filename < dups[j].Filename })
	return dups, nil
}

// FormatDuplicates renders duplicate reports for `park check`: one block per
// duplicated name, each copy tagged with the category holding it, followed by
// the manual repair instruction. Blocks are separated by a blank line.
func FormatDuplicates(dups []Duplicate) string {
	var b strings.Builder
	for i, d := range dups {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "duplicate: %s\n", d.Filename)
		width := 0
		for _, c := range d.Copies {
			width = max(width, len(c.Category))
		}
		for _, c := range d.Copies {
			fmt.Fprintf(&b, "  %-*s  %s\n", width, c.Category, c.Path)
		}
		b.WriteString("  rename or remove all but one copy\n")
	}
	return b.String()
}
