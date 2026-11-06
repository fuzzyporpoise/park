package store

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/note"
)

func TestInitCreatesFolders(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)

	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	for _, cat := range cfg.Categories {
		if _, err := os.Stat(cat.Path); err != nil {
			t.Errorf("missing category folder %q: %v", cat.Path, err)
		}
	}
}

func TestInitIdempotent(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)

	createdFirst, _, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() first error = %v", err)
	}
	if len(createdFirst) != len(cfg.Categories) {
		t.Fatalf("Init() first created = %d, want %d", len(createdFirst), len(cfg.Categories))
	}

	createdSecond, existedSecond, err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() second error = %v", err)
	}
	if len(createdSecond) != 0 {
		t.Fatalf("Init() second created = %d, want 0", len(createdSecond))
	}
	if len(existedSecond) != len(cfg.Categories) {
		t.Fatalf("Init() second existed = %d, want %d", len(existedSecond), len(cfg.Categories))
	}
}

func TestCheckAllExist(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	missing, err := Check(cfg)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(missing) > 0 {
		t.Fatalf("Check() = %v, want none", missing)
	}
}

func TestCheckMissing(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)

	// Only create one folder
	_ = os.MkdirAll(cfg.Categories[0].Path, 0o755)

	missing, err := Check(cfg)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(missing) != 3 {
		t.Fatalf("Check() = %d missing, want 3", len(missing))
	}
}

func TestCheckPathIsFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)

	// Create a file where a category folder is expected.
	if err := os.WriteFile(cfg.Categories[0].Path, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := Check(cfg)
	if err == nil {
		t.Fatal("Check() expected error when category path is a file")
	}
}

func TestNewCreatesNote(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{Filename: "My Note", Metadata: note.Metadata{Synopsis: "a synopsis", Source: "test", Category: "inbox"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("note file missing: %v", err)
	}

	n, err := note.Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if n.Category != "inbox" {
		t.Errorf("category = %q, want inbox", n.Category)
	}
	if n.Synopsis != "a synopsis" {
		t.Errorf("synopsis = %q, want %q", n.Synopsis, "a synopsis")
	}
	if n.Source != "test" {
		t.Errorf("source = %q, want test", n.Source)
	}
}

func TestReclassifyMovesFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{Filename: "Move Me", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	filename := filepath.Base(path)

	if err := Reclassify(cfg, filename, "projects"); err != nil {
		t.Fatalf("Reclassify() error = %v", err)
	}

	inboxPath := filepath.Join(tmp, "_inbox", filename)
	projectsPath := filepath.Join(tmp, "_projects", filename)

	if _, err := os.Stat(inboxPath); !os.IsNotExist(err) {
		t.Errorf("file still exists in inbox: %v", err)
	}
	if _, err := os.Stat(projectsPath); err != nil {
		t.Errorf("file missing in projects: %v", err)
	}

	n, err := note.Parse(projectsPath)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if n.Category != "projects" {
		t.Errorf("category = %q, want projects", n.Category)
	}
}

func TestReclassifySameCategory(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{Filename: "Stay Put", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	filename := filepath.Base(path)

	err = Reclassify(cfg, filename, "inbox")
	if err == nil {
		t.Fatal("expected error when reclassifying to the same category")
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("original file was moved or removed: %v", err)
	}
}

func TestReclassifyUnknownCategory(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)

	err := Reclassify(cfg, "note.md", "nope")
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
}

func TestReclassifyMissingFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	err := Reclassify(cfg, "missing.md", "projects")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReclassifyDestinationExists(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := note.Create(cfg, note.Draft{Filename: "Collision", Metadata: note.Metadata{Synopsis: "in inbox", Source: "test", Category: "inbox"}}); err != nil {
		t.Fatalf("Create() inbox error = %v", err)
	}
	// Planted directly rather than through Create: writes now reject a name
	// that already exists in another category, but duplicates predating that
	// rule can still be on disk and a move must not add to them.
	dup := filepath.Join(cfg.Categories[1].Path, "collision.md")
	if err := note.Write(dup, note.Note{Path: dup, Body: "# Collision\n", Metadata: note.Metadata{Category: "projects", Created: "2026-01-01", Source: "test", Synopsis: "in projects"}}); err != nil {
		t.Fatalf("write duplicate error = %v", err)
	}

	err := Reclassify(cfg, "collision.md", "projects")
	if err == nil {
		t.Fatal("expected error when destination file already exists")
	}
	if !strings.Contains(err.Error(), dup) {
		t.Errorf("error = %q, want it to name the colliding path %q", err, dup)
	}
}

func TestReclassifyByLiteralPath(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{Filename: "Literal Path", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	filename := filepath.Base(path)

	if err := Reclassify(cfg, path, "projects"); err != nil {
		t.Fatalf("Reclassify(%q) error = %v", path, err)
	}

	projectsPath := filepath.Join(tmp, "_projects", filename)
	if _, err := os.Stat(projectsPath); err != nil {
		t.Errorf("file missing in projects: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists in inbox: %v", err)
	}
}

func TestReclassifyByRelativePath(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{Filename: "Relative Path", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	filename := filepath.Base(path)

	t.Chdir(tmp)
	rel := filepath.Join("_inbox", filename)

	if err := Reclassify(cfg, rel, "projects"); err != nil {
		t.Fatalf("Reclassify(%q) error = %v", rel, err)
	}

	if _, err := os.Stat(filepath.Join(tmp, "_projects", filename)); err != nil {
		t.Errorf("file missing in projects: %v", err)
	}
}

func TestReclassifyRelativePathSameCategory(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{Filename: "Same Relative", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	filename := filepath.Base(path)

	t.Chdir(tmp)
	rel := filepath.Join("_inbox", filename)

	err = Reclassify(cfg, rel, "inbox")
	if err == nil {
		t.Fatal("expected error when reclassifying a relative path to the same category")
	}
	if !strings.Contains(err.Error(), "already in") {
		t.Errorf("error = %q, want already-in message", err.Error())
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("original file was moved or removed: %v", err)
	}
}

func TestScan(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := note.Create(cfg, note.Draft{Filename: "First", Metadata: note.Metadata{Synopsis: "oldest", Source: "test", Category: "inbox"}}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if _, err := note.Create(cfg, note.Draft{Filename: "Second", Metadata: note.Metadata{Synopsis: "newer", Source: "test", Category: "inbox"}}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	items, err := Scan(cfg, "inbox")
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Scan() returned %d items, want 2", len(items))
	}
}

func TestScanUnknownCategory(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)

	_, err := Scan(cfg, "nope")
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
}

func TestResolvePath(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{Filename: "Resolve Me", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	filename := filepath.Base(path)

	got, err := ResolvePath(cfg, filename)
	if err != nil {
		t.Fatalf("ResolvePath(%q) error = %v", filename, err)
	}
	if got != path {
		t.Errorf("ResolvePath(%q) = %q, want %q", filename, got, path)
	}

	fullPath, err := ResolvePath(cfg, path)
	if err != nil {
		t.Fatalf("ResolvePath(%q) error = %v", path, err)
	}
	if fullPath != path {
		t.Errorf("ResolvePath(%q) = %q, want %q", path, fullPath, path)
	}

	_, err = ResolvePath(cfg, "missing.md")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ResolvePath(missing) error = %v, want os.ErrNotExist", err)
	}
}

func TestResolvePathDoesNotDoubleJoin(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// A naive category join of "_inbox/nested.md" would land on this nested
	// file; a path-like argument must be treated as a literal path instead.
	nested := filepath.Join(cfg.Categories[0].Path, "_inbox")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "nested.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	_, err := ResolvePath(cfg, filepath.Join("_inbox", "nested.md"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ResolvePath(path) error = %v, want os.ErrNotExist (path must not double-join)", err)
	}
}

// writeRaw plants a note file directly, bypassing the write-time uniqueness
// check, so tests can reproduce stores that predate it.
func writeRaw(t *testing.T, dir, name, category string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	body := "---\ncategory: " + category + "\ncreated: 2026-01-01\nsource: test\nsynopsis: raw\n---\n\n# body\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
	return path
}

func TestLocate(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := note.Create(cfg, note.Draft{Filename: "Find Me", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "areas"}}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	got, err := note.Locate(cfg, "find-me.md")
	if err != nil {
		t.Fatalf("Locate() error = %v", err)
	}
	if len(got) != 1 || got[0] != filepath.Join(cfg.Categories[2].Path, "find-me.md") {
		t.Errorf("Locate() = %v, want the areas copy", got)
	}

	got, err = note.Locate(cfg, "nothing-here.md")
	if err != nil {
		t.Fatalf("Locate(missing) error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Locate(missing) = %v, want no matches", got)
	}

	// Legacy duplicates resolve in config order, which is why write paths
	// reject them rather than depending on that order.
	inboxDup := writeRaw(t, cfg.Categories[0].Path, "dup.md", "inbox")
	areasDup := writeRaw(t, cfg.Categories[2].Path, "dup.md", "areas")
	got, err = note.Locate(cfg, "dup.md")
	if err != nil {
		t.Fatalf("Locate(dup) error = %v", err)
	}
	if len(got) != 2 || got[0] != inboxDup || got[1] != areasDup {
		t.Errorf("Locate(dup) = %v, want [%s %s]", got, inboxDup, areasDup)
	}
}

func TestDuplicates(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if _, err := note.Create(cfg, note.Draft{Filename: "Unique", Metadata: note.Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"}}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	dups, err := Duplicates(cfg)
	if err != nil {
		t.Fatalf("Duplicates() error = %v", err)
	}
	if len(dups) != 0 {
		t.Fatalf("Duplicates() = %v, want none", dups)
	}

	inboxDup := writeRaw(t, cfg.Categories[0].Path, "dup.md", "inbox")
	archiveDup := writeRaw(t, cfg.Categories[3].Path, "dup.md", "archive")
	writeRaw(t, cfg.Categories[1].Path, "solo.md", "projects")

	dups, err = Duplicates(cfg)
	if err != nil {
		t.Fatalf("Duplicates() error = %v", err)
	}
	if len(dups) != 1 {
		t.Fatalf("Duplicates() = %v, want one entry", dups)
	}
	if dups[0].Filename != "dup.md" {
		t.Errorf("filename = %q, want dup.md", dups[0].Filename)
	}
	want := []DuplicateCopy{
		{Category: "inbox", Path: inboxDup},
		{Category: "archive", Path: archiveDup},
	}
	if !slices.Equal(dups[0].Copies, want) {
		t.Errorf("copies = %v, want %v", dups[0].Copies, want)
	}
}

func TestFormatDuplicates(t *testing.T) {
	if got := FormatDuplicates(nil); got != "" {
		t.Errorf("FormatDuplicates(nil) = %q, want empty", got)
	}

	got := FormatDuplicates([]Duplicate{
		{Filename: "dup.md", Copies: []DuplicateCopy{
			{Category: "inbox", Path: "/root/_inbox/dup.md"},
			{Category: "projects", Path: "/root/_projects/dup.md"},
		}},
		{Filename: "second.md", Copies: []DuplicateCopy{
			{Category: "areas", Path: "/root/_areas/second.md"},
			{Category: "archive", Path: "/root/_archive/second.md"},
		}},
	})

	for _, want := range []string{
		"duplicate: dup.md\n",
		"  inbox     /root/_inbox/dup.md\n",
		"  projects  /root/_projects/dup.md\n",
		"duplicate: second.md\n",
		"  areas    /root/_areas/second.md\n",
		"  archive  /root/_archive/second.md\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if count := strings.Count(got, "rename or remove all but one copy"); count != 2 {
		t.Errorf("repair hint appears %d times, want once per duplicate:\n%s", count, got)
	}
}
