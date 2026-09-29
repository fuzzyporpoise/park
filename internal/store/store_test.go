package store

import (
	"errors"
	"os"
	"path/filepath"
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
	if _, err := note.Create(cfg, note.Draft{Filename: "Collision", Metadata: note.Metadata{Synopsis: "in projects", Source: "test", Category: "projects"}}); err != nil {
		t.Fatalf("Create() projects error = %v", err)
	}

	err := Reclassify(cfg, "Collision.md", "projects")
	if err == nil {
		t.Fatal("expected error when destination file already exists")
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
