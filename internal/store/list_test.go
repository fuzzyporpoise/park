package store

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/note"
	"go.fuzzyporpoise.dev/park/schema"
)

// createNote parks a note in the given category and returns its filename.
func createNote(t *testing.T, cfg *config.Config, filename, category, synopsis string) string {
	t.Helper()
	path, err := note.Create(cfg, note.Draft{
		Filename: filename,
		Metadata: note.Metadata{Synopsis: synopsis, Source: "test", Category: category},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	return path
}

func TestListExcludesArchiveByDefault(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	createNote(t, cfg, "inbox-note", "inbox", "inbox item")
	createNote(t, cfg, "archive-note", "archive", "archive item")

	groups, err := List(cfg, ListOptions{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("List() returned %d groups, want 1", len(groups))
	}
	if groups[0].Category != "inbox" {
		t.Errorf("group category = %q, want inbox", groups[0].Category)
	}
}

func TestListIncludeExcluded(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	createNote(t, cfg, "inbox-note", "inbox", "inbox item")
	createNote(t, cfg, "archive-note", "archive", "archive item")

	groups, err := List(cfg, ListOptions{IncludeExcluded: true})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("List() returned %d groups, want 2", len(groups))
	}
	if groups[0].Category != "inbox" || groups[1].Category != "archive" {
		t.Errorf("groups = %q, %q; want inbox, archive (config order)", groups[0].Category, groups[1].Category)
	}
}

func TestListExplicitCategoryOverridesExclusion(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	createNote(t, cfg, "archive-note", "archive", "archive item")

	groups, err := List(cfg, ListOptions{Categories: []string{"archive"}})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Category != "archive" {
		t.Fatalf("List() = %+v, want a single archive group", groups)
	}
}

func TestListUnknownCategory(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)

	_, err := List(cfg, ListOptions{Categories: []string{"nope"}})
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
	if !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("error = %q, want unknown-category message", err.Error())
	}
}

func TestListOmitsEmptyCategories(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	createNote(t, cfg, "area-note", "areas", "area item")

	groups, err := List(cfg, ListOptions{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(groups) != 1 || groups[0].Category != "areas" {
		t.Fatalf("List() = %+v, want a single areas group", groups)
	}
}

func TestFormatList(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	createNote(t, cfg, "first-note", "inbox", "a synopsis")

	groups, err := List(cfg, ListOptions{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var b bytes.Buffer
	if err := FormatList(&b, groups); err != nil {
		t.Fatalf("FormatList() error = %v", err)
	}
	out := b.String()
	for _, want := range []string{
		"INBOX\n-----\n",
		"filename: first-note.md\n",
		"category: inbox\n",
		"source: test\n",
		"synopsis: a synopsis\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatList() output missing %q:\n%s", want, out)
		}
	}
}

func TestFormatListSeparatesCategories(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	createNote(t, cfg, "inbox-note", "inbox", "inbox item")
	createNote(t, cfg, "projects-note", "projects", "projects item")

	groups, err := List(cfg, ListOptions{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var b bytes.Buffer
	if err := FormatList(&b, groups); err != nil {
		t.Fatalf("FormatList() error = %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "\n\nPROJECTS\n--------\n") {
		t.Errorf("FormatList() should separate categories with a blank line:\n%s", out)
	}
	if strings.Contains(out, "\n--------\nPROJECTS") {
		t.Errorf("FormatList() emitted a top divider rule:\n%s", out)
	}
}

func TestFormatListEmpty(t *testing.T) {
	var b bytes.Buffer
	if err := FormatList(&b, nil); err != nil {
		t.Fatalf("FormatList() error = %v", err)
	}
	if got := strings.TrimSpace(b.String()); got != "No notes found." {
		t.Errorf("FormatList() = %q, want %q", got, "No notes found.")
	}
}

func TestWriteListJSON(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	createNote(t, cfg, "json-note", "inbox", "json synopsis")

	groups, err := List(cfg, ListOptions{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	var b bytes.Buffer
	if err := WriteListJSON(&b, groups); err != nil {
		t.Fatalf("WriteListJSON() error = %v", err)
	}

	var env ListEnvelope
	if err := json.Unmarshal(b.Bytes(), &env); err != nil {
		t.Fatalf("parse JSON = %v\n%s", err, b.String())
	}
	if env.SchemaVersion != schema.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", env.SchemaVersion, schema.SchemaVersion)
	}
	if len(env.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(env.Items))
	}
	item := env.Items[0]
	if item.Filename != "json-note.md" {
		t.Errorf("filename = %q, want json-note.md", item.Filename)
	}
	if item.Category != "inbox" {
		t.Errorf("category = %q, want inbox", item.Category)
	}
	if item.Source != "test" {
		t.Errorf("source = %q, want test", item.Source)
	}
	if item.Synopsis != "json synopsis" {
		t.Errorf("synopsis = %q, want json synopsis", item.Synopsis)
	}
	if item.Created == "" {
		t.Error("created is empty, want a date")
	}
	if _, err := time.Parse(time.RFC3339, item.Modified); err != nil {
		t.Errorf("modified = %q, not RFC3339: %v", item.Modified, err)
	}
	if !strings.HasSuffix(item.Modified, "Z") {
		t.Errorf("modified = %q, want UTC (Z suffix)", item.Modified)
	}
}

func TestWriteListJSONEmpty(t *testing.T) {
	var b bytes.Buffer
	if err := WriteListJSON(&b, nil); err != nil {
		t.Fatalf("WriteListJSON() error = %v", err)
	}
	if !strings.Contains(b.String(), `"items": []`) {
		t.Errorf("empty JSON = %s, want an empty items array", b.String())
	}
}
