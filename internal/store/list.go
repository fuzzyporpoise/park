package store

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/schema"
)

// ListOptions controls which categories List scans.
type ListOptions struct {
	// IncludeExcluded includes categories marked Excluded in the config. It is
	// ignored when Categories is set, since naming a category explicitly
	// overrides its excluded flag.
	IncludeExcluded bool
	// Categories restricts the listing to the named categories. When empty,
	// every category except those marked Excluded is listed. Naming a
	// category explicitly overrides its excluded flag.
	Categories []string
}

// Group is the set of notes in a single category.
type Group struct {
	Category string
	Items    []Item
}

// selectedCategories resolves the categories to scan, in config order and
// deduplicated. It returns an error naming the first unknown category in
// opts.Categories.
func selectedCategories(cfg *config.Config, opts ListOptions) ([]config.Category, error) {
	if len(opts.Categories) == 0 {
		var out []config.Category
		for _, cl := range cfg.Categories {
			if cl.Excluded && !opts.IncludeExcluded {
				continue
			}
			out = append(out, cl)
		}
		return out, nil
	}

	want := make(map[string]struct{}, len(opts.Categories))
	for _, name := range opts.Categories {
		if _, ok := cfg.CategoryByName(name); !ok {
			return nil, cfg.UnknownCategoryError(name)
		}
		want[name] = struct{}{}
	}

	var out []config.Category
	for _, cl := range cfg.Categories {
		if _, ok := want[cl.Name]; ok {
			out = append(out, cl)
		}
	}
	return out, nil
}

// List returns parked notes grouped by category according to opts. Groups
// preserve config category order and empty groups are omitted; notes within a
// group are ordered by Scan (oldest modified first).
func List(cfg *config.Config, opts ListOptions) ([]Group, error) {
	cats, err := selectedCategories(cfg, opts)
	if err != nil {
		return nil, err
	}

	var groups []Group
	for _, cl := range cats {
		items, scanErr := Scan(cfg, cl.Name)
		if scanErr != nil {
			return nil, scanErr
		}
		if len(items) == 0 {
			continue
		}
		groups = append(groups, Group{Category: cl.Name, Items: items})
	}
	return groups, nil
}

// FormatList writes the human-readable listing in a frontmatter-like layout: an
// upper-cased category banner followed by one block per note listing its
// filename and frontmatter fields. Groups with no notes are omitted, and a
// listing with nothing to show prints a single notice.
func FormatList(w io.Writer, groups []Group) error {
	if len(groups) == 0 {
		if _, err := fmt.Fprintln(w, "No notes found."); err != nil {
			return fmt.Errorf("write list output: %w", err)
		}
		return nil
	}

	for i, g := range groups {
		header := strings.ToUpper(g.Category)
		rule := strings.Repeat("-", len(header))
		if _, err := fmt.Fprintf(w, "%s\n%s\n", header, rule); err != nil {
			return fmt.Errorf("write list output: %w", err)
		}
		for _, it := range g.Items {
			if _, err := fmt.Fprintf(w, "\nfilename: %s\ncategory: %s\ncreated: %s\nsource: %s\nsynopsis: %s\n",
				it.Filename, g.Category, it.Created, it.Source, it.Synopsis); err != nil {
				return fmt.Errorf("write list output: %w", err)
			}
		}
		if i < len(groups)-1 {
			if _, err := fmt.Fprintln(w); err != nil {
				return fmt.Errorf("write list output: %w", err)
			}
		}
	}
	return nil
}

// ListItem is the machine-readable representation of one parked note.
type ListItem struct {
	Filename string `json:"filename"`
	Path     string `json:"path"`
	Category string `json:"category"`
	Created  string `json:"created"`
	Source   string `json:"source"`
	Synopsis string `json:"synopsis"`
	Modified string `json:"modified"`
}

// ListEnvelope is the versioned JSON contract for `park list --json`.
type ListEnvelope struct {
	SchemaVersion int        `json:"schema_version"`
	Items         []ListItem `json:"items"`
}

// WriteListJSON writes the notes as a flat, versioned JSON envelope. Every
// group is flattened into a single items array and each item carries its
// category. Items is always emitted as an array, never null.
func WriteListJSON(w io.Writer, groups []Group) error {
	items := make([]ListItem, 0)
	for _, g := range groups {
		for _, it := range g.Items {
			items = append(items, ListItem{
				Filename: it.Filename,
				Path:     it.Path,
				Category: g.Category,
				Created:  it.Created,
				Source:   it.Source,
				Synopsis: it.Synopsis,
				Modified: it.ModTime.UTC().Format(time.RFC3339),
			})
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(ListEnvelope{SchemaVersion: schema.SchemaVersion, Items: items}); err != nil {
		return fmt.Errorf("encode list json: %w", err)
	}
	return nil
}
