package render

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/note"
)

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*[a-zA-Z]")

func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}

func TestShowFileRendersNote(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if err := os.MkdirAll(cfg.Categories[0].Path, 0o755); err != nil {
		t.Fatalf("create category folder: %v", err)
	}

	path := filepath.Join(cfg.Categories[0].Path, "test-note.md")
	n := note.Note{
		Body: "# Hello\n\nbody content\n",
		Metadata: note.Metadata{
			Category: "inbox",
			Created:  "2026-08-09",
			Source:   "terminal",
			Synopsis: "a rendered note",
		},
	}
	if err := note.Write(path, n); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var buf bytes.Buffer
	if err := ShowFile(path, &buf, false); err != nil {
		t.Fatalf("ShowFile() error = %v", err)
	}

	out := buf.String()
	if out == "" {
		t.Fatal("ShowFile() produced empty output")
	}
	plain := stripANSI(out)
	if !strings.Contains(plain, "a rendered note") {
		t.Errorf("output missing synopsis; got %q", plain)
	}
	if !strings.Contains(plain, "Hello") {
		t.Errorf("output missing body; got %q", plain)
	}
}

func TestShowFileMissing(t *testing.T) {
	var buf bytes.Buffer
	err := ShowFile(filepath.Join(t.TempDir(), "missing.md"), &buf, false)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestShowFilePlain(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if err := os.MkdirAll(cfg.Categories[0].Path, 0o755); err != nil {
		t.Fatalf("create category folder: %v", err)
	}

	path := filepath.Join(cfg.Categories[0].Path, "plain-note.md")
	n := note.Note{
		Body: "# Hello\n\nbody content\n",
		Metadata: note.Metadata{
			Category: "inbox",
			Created:  "2026-08-09",
			Source:   "terminal",
			Synopsis: "a rendered note",
		},
	}
	if err := note.Write(path, n); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	var buf bytes.Buffer
	if err := ShowFile(path, &buf, true); err != nil {
		t.Fatalf("ShowFile() error = %v", err)
	}

	out := buf.String()
	if ansiEscape.MatchString(out) {
		t.Errorf("plain output contains ANSI escapes: %q", out)
	}
	for _, want := range []string{"a rendered note", "body content", "category: inbox"} {
		if !strings.Contains(out, want) {
			t.Errorf("plain output missing %q; got %q", want, out)
		}
	}
	if strings.Contains(out, "&nbsp;") {
		t.Errorf("plain output contains literal &nbsp;: %q", out)
	}
}
