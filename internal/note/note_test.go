package note

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.fuzzyporpoise.dev/park/internal/config"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		want     Metadata
		wantBody string
	}{
		{
			name:    "all fields",
			content: "---\ncategory: inbox\ncreated: 2026-07-28\nsource: terminal\nsynopsis: a test note\n---\n\n# hello\n",
			want: Metadata{
				Category: "inbox",
				Created:  "2026-07-28",
				Source:   "terminal",
				Synopsis: "a test note",
			},
			wantBody: "# hello",
		},
		{
			name:    "empty body",
			content: "---\ncategory: archive\ncreated: 2026-07-28\nsource: chat\nsynopsis:\n---\n",
			want: Metadata{
				Category: "archive",
				Created:  "2026-07-28",
				Source:   "chat",
				Synopsis: "",
			},
			wantBody: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmp := t.TempDir()
			path := filepath.Join(tmp, "note.md")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("write test file: %v", err)
			}

			got, err := Parse(path)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}
			if got.Path != path {
				t.Errorf("Parse() path = %q, want %q", got.Path, path)
			}
			if got.Category != tt.want.Category || got.Created != tt.want.Created || got.Source != tt.want.Source || got.Synopsis != tt.want.Synopsis {
				t.Errorf("Parse() = %+v, want %+v", got.Metadata, tt.want)
			}
			if got.Body != tt.wantBody {
				t.Errorf("Parse() body = %q, want %q", got.Body, tt.wantBody)
			}
		})
	}
}

func TestParseMissingFile(t *testing.T) {
	_, err := Parse(filepath.Join(t.TempDir(), "nope.md"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWriteRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "note.md")
	n := Note{
		Metadata: Metadata{
			Category: "projects",
			Created:  "2026-07-28",
			Source:   "test",
			Synopsis: "round trip",
		},
		Body: "# title\n",
	}

	if err := Write(path, n); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	got, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Metadata != n.Metadata {
		t.Errorf("frontmatter mismatch: %+v, want %+v", got.Metadata, n.Metadata)
	}
	if got.Body != "# title\n" {
		t.Errorf("body = %q, want %q", got.Body, "# title\n")
	}
}

func TestToday(t *testing.T) {
	got := Today()
	if len(got) != 10 {
		t.Errorf("Today() = %q, expected YYYY-MM-DD format", got)
	}
}

func TestCreate(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if err := os.MkdirAll(cfg.Categories[0].Path, 0o755); err != nil {
		t.Fatalf("create category folder: %v", err)
	}

	t.Run("creates note with body", func(t *testing.T) {
		body := "## heading\n\nparagraph\n"
		path, err := Create(cfg, Draft{
			Filename: "Body Note",
			Body:     body,
			Metadata: Metadata{Synopsis: "with body", Source: "test", Category: "inbox"},
		})
		if err != nil {
			t.Fatalf("Create() error = %v", err)
		}

		got, err := Parse(path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Category != "inbox" {
			t.Errorf("category = %q, want inbox", got.Category)
		}
		if got.Body != body {
			t.Errorf("body = %q, want %q", got.Body, body)
		}
	})

	t.Run("missing folder returns error", func(t *testing.T) {
		cfgNoFolder := config.DefaultConfig(t.TempDir())
		_, err := Create(cfgNoFolder, Draft{
			Filename: "Note",
			Metadata: Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"},
		})
		if err == nil {
			t.Fatal("expected error when category folder does not exist")
		}
	})

	t.Run("duplicate slug returns error", func(t *testing.T) {
		_, err := Create(cfg, Draft{
			Filename: "Duplicate",
			Metadata: Metadata{Synopsis: "first", Source: "test", Category: "inbox"},
		})
		if err != nil {
			t.Fatalf("first create: %v", err)
		}
		_, err = Create(cfg, Draft{
			Filename: "Duplicate",
			Metadata: Metadata{Synopsis: "second", Source: "test", Category: "inbox"},
		})
		if err == nil {
			t.Fatal("expected error for duplicate slug")
		}
	})

}

func TestCreateFromFile(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if err := os.MkdirAll(cfg.Categories[1].Path, 0o755); err != nil {
		t.Fatalf("create category folder: %v", err)
	}

	src := filepath.Join(tmp, "draft.md")
	content := "# Draft\n\nsome content\n"
	if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	d, err := IngestFile(Draft{
		Filename: "Draft Note",
		FromFile: src,
		Metadata: Metadata{Synopsis: "ingested", Source: "agent", Category: "projects"},
	})
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}

	path, err := Create(cfg, d)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source file was not removed: %v", err)
	}

	got, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if got.Category != "projects" {
		t.Errorf("category = %q, want projects", got.Category)
	}
	if got.Synopsis != "ingested" {
		t.Errorf("synopsis = %q, want ingested", got.Synopsis)
	}
	if got.Source != "agent" {
		t.Errorf("source = %q, want agent", got.Source)
	}
	wantBody := strings.TrimRight(content, "\n")
	if got.Body != wantBody {
		t.Errorf("body = %q, want %q", got.Body, wantBody)
	}
}

func TestIngestFileMissingSource(t *testing.T) {
	tmp := t.TempDir()

	_, err := IngestFile(Draft{
		Filename: "Filename",
		FromFile: filepath.Join(tmp, "missing.md"),
		Metadata: Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"},
	})
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestIngestFileDirectorySource(t *testing.T) {
	tmp := t.TempDir()

	dir := filepath.Join(tmp, "adir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}

	_, err := IngestFile(Draft{
		Filename: "Filename",
		FromFile: dir,
		Metadata: Metadata{Synopsis: "synopsis", Source: "test", Category: "inbox"},
	})
	if err == nil {
		t.Fatal("expected error for directory source path")
	}
}

func TestIngestFileExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory available")
	}
	src, err := os.CreateTemp(home, "park-tilde-test-*.md")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(src.Name()) })
	if _, err := src.WriteString("# Tilde\n\nbody\n"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	d, err := IngestFile(Draft{FromFile: "~/" + filepath.Base(src.Name())})
	if err != nil {
		t.Fatalf("IngestFile() error = %v", err)
	}
	if d.FromFile != src.Name() {
		t.Errorf("FromFile = %q, want expanded %q", d.FromFile, src.Name())
	}
	if d.Body != "# Tilde\n\nbody" {
		t.Errorf("body = %q, want file body", d.Body)
	}
}

func TestDraftH1(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantHeading string
		wantOK      bool
	}{
		{"h1 at start", "# My Title\n\nbody content\n", "My Title", true},
		{"h1 after blank lines", "\n\n# Another Title\ncontent\n", "Another Title", true},
		{"no h1", "just some text\n", "", false},
		{"h2 not extracted", "## Section\n", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Draft{Body: tt.body}
			heading, ok := d.H1()
			if ok != tt.wantOK {
				t.Fatalf("H1() ok = %v, want %v", ok, tt.wantOK)
			}
			if heading != tt.wantHeading {
				t.Errorf("H1() heading = %q, want %q", heading, tt.wantHeading)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	for i := range cfg.Categories {
		if err := os.MkdirAll(cfg.Categories[i].Path, 0o755); err != nil {
			t.Fatalf("create category folder: %v", err)
		}
	}

	t.Run("all metadata no body creates note", func(t *testing.T) {
		d := Draft{
			Filename: "test-note",
			Metadata: Metadata{Synopsis: "a test", Source: "terminal", Category: "inbox"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		if out.Path == "" {
			t.Fatal("expected path, got empty")
		}
		got, err := Parse(out.Path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Synopsis != "a test" || got.Source != "terminal" || got.Category != "inbox" {
			t.Errorf("frontmatter mismatch: %+v", got.Metadata)
		}
	})

	t.Run("file with all metadata creates directly", func(t *testing.T) {
		src := filepath.Join(tmp, "body.md")
		content := "## Body\n\ncontent\n"
		if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		d := Draft{
			Filename: "from-file",
			FromFile: src,
			Metadata: Metadata{Synopsis: "from file", Source: "migration", Category: "archive"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Errorf("source file was not removed")
		}
		got, err := Parse(out.Path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Category != "archive" || got.Synopsis != "from file" || got.Source != "migration" {
			t.Errorf("frontmatter mismatch: %+v", got.Metadata)
		}
		wantBody := strings.TrimRight(content, "\n")
		if got.Body != wantBody {
			t.Errorf("body = %q, want %q", got.Body, wantBody)
		}
	})

	t.Run("from-file and body together returns error", func(t *testing.T) {
		src := filepath.Join(tmp, "conflict.md")
		if err := os.WriteFile(src, []byte("body from file\n"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		d := Draft{
			Filename: "conflict",
			FromFile: src,
			Body:     "body from stdin\n",
			Metadata: Metadata{Synopsis: "conflict", Source: "test", Category: "inbox"},
		}
		_, err := Add(cfg, d)
		if err == nil {
			t.Fatal("expected error when both --from-file and body are provided")
		}
	})

	t.Run("stdin body without frontmatter and all metadata creates directly", func(t *testing.T) {
		d := Draft{
			Filename: "piped",
			Body:     "# Piped title\n\ntext\n",
			Metadata: Metadata{Synopsis: "piped body", Source: "stdin", Category: "projects"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		got, err := Parse(out.Path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Category != "projects" {
			t.Errorf("category = %q, want projects", got.Category)
		}
		if got.Body != "# Piped title\n\ntext\n" {
			t.Errorf("body = %q, want %q", got.Body, "# Piped title\n\ntext\n")
		}
	})

	t.Run("body with frontmatter uses frontmatter values", func(t *testing.T) {
		d := Draft{
			Body: "---\ncategory: areas\nsource: chat\nsynopsis: fm-driven\ncreated: 2026-07-01\n---\n\n# Title\n\nbody\n",
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		got, err := Parse(out.Path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Category != "areas" || got.Source != "chat" || got.Synopsis != "fm-driven" {
			t.Errorf("frontmatter mismatch: %+v", got.Metadata)
		}
	})

	t.Run("body with frontmatter and explicit metadata uses explicit metadata", func(t *testing.T) {
		d := Draft{
			Filename: "explicit",
			Body:     "---\ncategory: areas\nsource: chat\nsynopsis: fm-driven\ncreated: 2026-07-01\n---\n\n# Title\n\nbody\n",
			Metadata: Metadata{Synopsis: "explicit", Source: "explicit", Category: "archive"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		got, err := Parse(out.Path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Category != "archive" || got.Source != "explicit" || got.Synopsis != "explicit" {
			t.Errorf("frontmatter mismatch: %+v", got.Metadata)
		}
		if strings.Contains(got.Body, "fm-driven") {
			t.Errorf("body still contains old frontmatter")
		}
	})

	t.Run("missing metadata without body returns form", func(t *testing.T) {
		d := Draft{Filename: "only-filename"}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form == nil {
			t.Fatal("expected form outcome")
		}
	})

	t.Run("body with incomplete frontmatter returns error", func(t *testing.T) {
		d := Draft{
			Body: "---\ncategory: inbox\n---\n\nbody\n",
		}
		_, err := Add(cfg, d)
		if err == nil {
			t.Fatal("expected error for incomplete frontmatter")
		}
	})

	t.Run("body with incomplete frontmatter and CLI metadata creates directly", func(t *testing.T) {
		d := Draft{
			Filename: "merged",
			Body:     "---\ncategory: inbox\n---\n\n# Title\n\nbody\n",
			Metadata: Metadata{Synopsis: "cli synopsis", Source: "cli"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		got, err := Parse(out.Path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Category != "inbox" || got.Source != "cli" || got.Synopsis != "cli synopsis" {
			t.Errorf("frontmatter mismatch: %+v", got.Metadata)
		}
		if strings.Contains(got.Body, "category:") {
			t.Errorf("body still contains frontmatter")
		}
	})

	t.Run("from-file without filename uses source basename", func(t *testing.T) {
		src := filepath.Join(tmp, "rando-file.md")
		content := "## Random\n\ncontent\n"
		if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		d := Draft{
			FromFile: src,
			Metadata: Metadata{Synopsis: "from source file", Source: "migration", Category: "archive"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		if !strings.HasSuffix(out.Path, "rando-file.md") {
			t.Errorf("path = %q, expected suffix rando-file.md", out.Path)
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Errorf("source file was not removed")
		}
	})

	t.Run("from-file with missing metadata returns form with filename populated", func(t *testing.T) {
		src := filepath.Join(tmp, "draft-note.md")
		content := "plain body without frontmatter\n"
		if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		d := Draft{
			FromFile: src,
			Metadata: Metadata{Source: "migration"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form == nil {
			t.Fatal("expected form outcome")
		}
		if out.Form.Filename != "draft-note.md" {
			t.Errorf("form filename = %q, want draft-note.md", out.Form.Filename)
		}
		if out.Form.Source != "migration" {
			t.Errorf("form source = %q, want migration", out.Form.Source)
		}
	})

	t.Run("stdin body without frontmatter and missing metadata returns form with body", func(t *testing.T) {
		body := "# Piped Body\n\ntext\n"
		d := Draft{Body: body}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form == nil {
			t.Fatal("expected form outcome")
		}
		if out.Form.Body != body {
			t.Errorf("form body = %q, want %q", out.Form.Body, body)
		}
		if out.Form.Category != "inbox" {
			t.Errorf("form category = %q, want inbox", out.Form.Category)
		}
		if out.Form.Filename != "Piped Body" {
			t.Errorf("form filename = %q, want %q", out.Form.Filename, "Piped Body")
		}
	})

	t.Run("body with H1 and no filename derives filename", func(t *testing.T) {
		d := Draft{
			Body:     "# Derived Title\n\nbody\n",
			Metadata: Metadata{Synopsis: "h1 derived", Source: "stdin", Category: "inbox"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		if !strings.HasSuffix(out.Path, "derived-title.md") {
			t.Errorf("path = %q, expected suffix derived-title.md", out.Path)
		}
	})

	t.Run("body without H1 and no filename returns error", func(t *testing.T) {
		d := Draft{
			Body:     "just text without a heading\n",
			Metadata: Metadata{Synopsis: "no h1", Source: "stdin", Category: "inbox"},
		}
		_, err := Add(cfg, d)
		if err == nil {
			t.Fatal("expected error when no filename and no H1")
		}
	})

	t.Run("body with frontmatter and explicit category flag overrides category", func(t *testing.T) {
		d := Draft{
			Body:     "---\ncategory: areas\nsource: chat\nsynopsis: fm-driven\ncreated: 2026-07-01\n---\n\n# Title\n\nbody\n",
			Metadata: Metadata{Category: "archive"},
		}
		out, err := Add(cfg, d)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
		if out.Form != nil {
			t.Fatal("expected direct creation, got form")
		}
		got, err := Parse(out.Path)
		if err != nil {
			t.Fatalf("Parse() error = %v", err)
		}
		if got.Category != "archive" || got.Source != "chat" || got.Synopsis != "fm-driven" {
			t.Errorf("frontmatter mismatch: %+v", got.Metadata)
		}
	})
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "Book Todo Legacy", "book-todo-legacy"},
		{"already slug", "book-todo-legacy", "book-todo-legacy"},
		{"with markdown extension", "book-todo-legacy.md", "book-todo-legacy"},
		{"with uppercase markdown extension", "book-todo-legacy.MD", "book-todo-legacy"},
		{"mixed case extension", "book-todo-legacy.Md", "book-todo-legacy"},
		{"with date", "Book Todo Legacy 2026-08-06", "book-todo-legacy-2026-08-06"},
		{"with date and extension", "Book Todo Legacy 2026-08-06.md", "book-todo-legacy-2026-08-06"},
		{"trailing spaces", "  book-todo-legacy.md  ", "book-todo-legacy"},
		{"empty becomes empty", "", ""},
		{"only extension", ".md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugify(tt.in)
			if got != tt.want {
				t.Errorf("slugify(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
