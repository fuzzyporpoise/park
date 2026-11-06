package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/urfave/cli/v3"
	"go.fuzzyporpoise.dev/park/schema"
)

// runPark builds a fresh command rooted in a temp directory and runs it with
// the supplied args, capturing stdout and stderr. It overrides the
// ExitErrHandler so exit-code errors are returned instead of calling os.Exit.
func runPark(t *testing.T, root string, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	t.Setenv("PARK_ROOT", root)
	t.Setenv("PARK_CONFIG", filepath.Join(root, "config"))

	cmd := newCommand()
	var out, errOut strings.Builder
	cmd.Writer = &out
	cmd.ErrWriter = &errOut
	cmd.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	full := append([]string{"park"}, args...)
	err = cmd.Run(context.Background(), full)
	return out.String(), errOut.String(), err
}

func exitCode(t *testing.T, err error) int {
	t.Helper()
	var c cli.ExitCoder
	if !errors.As(err, &c) {
		t.Fatalf("error %v does not implement cli.ExitCoder", err)
	}
	return c.ExitCode()
}

func TestConfigCommand(t *testing.T) {
	out, _, err := runPark(t, t.TempDir(), "config")
	if err != nil {
		t.Fatalf("config command error = %v", err)
	}
	for _, want := range []string{"default_category", "inbox", "projects", "areas", "archive"} {
		if !strings.Contains(out, want) {
			t.Errorf("config output missing %q:\n%s", want, out)
		}
	}
}

func TestInitCommand(t *testing.T) {
	root := t.TempDir()
	out, _, err := runPark(t, root, "init")
	if err != nil {
		t.Fatalf("init command error = %v", err)
	}
	if !strings.Contains(out, "created park folders") {
		t.Errorf("init output = %q, want created-park-folders message", out)
	}
	for _, name := range []string{"_inbox", "_projects", "_areas", "_archive"} {
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			t.Errorf("expected folder %q to exist: %v", name, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory", name)
		}
	}
}

func TestReclassifyMissingArgs(t *testing.T) {
	_, _, err := runPark(t, t.TempDir(), "reclassify")
	if err == nil {
		t.Fatal("expected error for reclassify without a file argument")
	}
	if got := exitCode(t, err); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
}

func TestReclassifyUnknownCategory(t *testing.T) {
	_, _, err := runPark(t, t.TempDir(), "reclassify", "somefile.md", "--category", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
	if got := exitCode(t, err); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
	if !strings.Contains(err.Error(), "unknown category") {
		t.Errorf("error = %q, want unknown-category message", err.Error())
	}
}

func TestReclassifySameCategory(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "_inbox")
	if err := os.MkdirAll(inbox, 0o755); err != nil {
		t.Fatalf("create inbox: %v", err)
	}
	notePath := filepath.Join(inbox, "existing.md")
	content := "---\ncategory: inbox\ncreated: 2026-01-01\nsource: test\nsynopsis: test\n---\n\nbody\n"
	if err := os.WriteFile(notePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	_, _, err := runPark(t, root, "reclassify", "existing.md", "--category", "inbox")
	if err == nil {
		t.Fatal("expected error for same-category reclassify")
	}
	if got := exitCode(t, err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
	if !strings.Contains(err.Error(), "already in") {
		t.Errorf("error = %q, want already-in message", err.Error())
	}
}

func TestReclassifyAcceptsLiteralPath(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	notePath := filepath.Join(root, "_inbox", "path-note.md")
	writeNote(t, filepath.Join(root, "_inbox"), "path-note.md", "inbox", "a path note")

	if _, _, err := runPark(t, root, "reclassify", notePath, "--category", "projects"); err != nil {
		t.Fatalf("reclassify by literal path error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "_projects", "path-note.md")); err != nil {
		t.Errorf("file missing in projects: %v", err)
	}
	if _, err := os.Stat(notePath); !os.IsNotExist(err) {
		t.Errorf("file still exists in inbox: %v", err)
	}
}

func TestReclassifyAcceptsRelativePath(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "rel-note.md", "inbox", "a relative note")

	t.Chdir(root)
	if _, _, err := runPark(t, root, "reclassify", filepath.Join("_inbox", "rel-note.md"), "--category", "areas"); err != nil {
		t.Fatalf("reclassify by relative path error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "_areas", "rel-note.md")); err != nil {
		t.Errorf("file missing in areas: %v", err)
	}
}

func TestShowMissingArg(t *testing.T) {
	_, _, err := runPark(t, t.TempDir(), "show")
	if err == nil {
		t.Fatal("expected error for show without a file argument")
	}
	if got := exitCode(t, err); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
}

func TestShowMissingFile(t *testing.T) {
	_, _, err := runPark(t, t.TempDir(), "show", "nonexistent.md")
	if err == nil {
		t.Fatal("expected error for show of a missing file")
	}
	if got := exitCode(t, err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
}

func TestShowPlainFlag(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "plain-note.md", "inbox", "a plain note")

	out, _, err := runPark(t, root, "show", "plain-note.md", "--plain")
	if err != nil {
		t.Fatalf("show --plain error = %v", err)
	}
	assertPlainShow(t, out, "a plain note")
}

// TestShowNonTTYIsPlain verifies plain output is auto-selected when the
// writer is not a terminal; runPark wires a strings.Builder as cmd.Writer,
// which writerIsTTY treats as non-terminal.
func TestShowNonTTYIsPlain(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "piped-note.md", "inbox", "a piped note")

	out, _, err := runPark(t, root, "show", "piped-note.md")
	if err != nil {
		t.Fatalf("show error = %v", err)
	}
	assertPlainShow(t, out, "a piped note")
}

func assertPlainShow(t *testing.T, out, synopsis string) {
	t.Helper()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("plain show output contains ANSI escapes:\n%q", out)
	}
	for _, want := range []string{synopsis, "body"} {
		if !strings.Contains(out, want) {
			t.Errorf("show output missing %q:\n%s", want, out)
		}
	}
}

func TestStyledExit(t *testing.T) {
	err := styledExit(errors.New("boom"), 7)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if got := exitCode(t, err); got != 7 {
		t.Errorf("exit code = %d, want 7", got)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %q, want boom message", err.Error())
	}
}

// writeNote writes a minimal valid note into the given category folder.
func writeNote(t *testing.T, dir, name, category, synopsis string) {
	t.Helper()
	content := "---\ncategory: " + category + "\ncreated: 2026-01-01\nsource: test\nsynopsis: " + synopsis + "\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write note %q: %v", name, err)
	}
}

func TestListCommandDefaultExcludesArchive(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "inbox-note.md", "inbox", "an inbox item")
	writeNote(t, filepath.Join(root, "_archive"), "archive-note.md", "archive", "an archived item")

	out, _, err := runPark(t, root, "list")
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if !strings.Contains(out, "inbox-note.md") {
		t.Errorf("list output missing inbox note:\n%s", out)
	}
	if strings.Contains(out, "archive-note.md") {
		t.Errorf("list output included excluded archive:\n%s", out)
	}
}

func TestListAllIncludesArchive(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_archive"), "archive-note.md", "archive", "an archived item")

	out, _, err := runPark(t, root, "list", "--all")
	if err != nil {
		t.Fatalf("list --all error = %v", err)
	}
	if !strings.Contains(out, "archive-note.md") {
		t.Errorf("list --all output missing archive note:\n%s", out)
	}
}

func TestListCategoryOverridesExclusion(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_archive"), "archive-note.md", "archive", "an archived item")

	out, _, err := runPark(t, root, "list", "--category", "archive")
	if err != nil {
		t.Fatalf("list --category error = %v", err)
	}
	if !strings.Contains(out, "archive-note.md") {
		t.Errorf("list --category archive output missing archive note:\n%s", out)
	}
}

func TestListCategoryUnknown(t *testing.T) {
	_, _, err := runPark(t, t.TempDir(), "list", "--category", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
	if got := exitCode(t, err); got != 2 {
		t.Errorf("exit code = %d, want 2", got)
	}
}

func TestListJSON(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "json-note.md", "inbox", "a json item")

	out, _, err := runPark(t, root, "list", "--json")
	if err != nil {
		t.Fatalf("list --json error = %v", err)
	}

	var env struct {
		SchemaVersion int `json:"schema_version"`
		Items         []struct {
			Filename string `json:"filename"`
			Category string `json:"category"`
			Synopsis string `json:"synopsis"`
			Modified string `json:"modified"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("parse list json = %v\n%s", err, out)
	}
	if env.SchemaVersion != schema.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", env.SchemaVersion, schema.SchemaVersion)
	}
	if len(env.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(env.Items))
	}
	if env.Items[0].Filename != "json-note.md" || env.Items[0].Category != "inbox" {
		t.Errorf("item = %+v, want json-note.md in inbox", env.Items[0])
	}
	if env.Items[0].Synopsis != "a json item" {
		t.Errorf("synopsis = %q, want %q", env.Items[0].Synopsis, "a json item")
	}
	if env.Items[0].Modified == "" {
		t.Error("modified is empty, want a timestamp")
	}
}

func TestListJSONCategoryFilter(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "inbox-note.md", "inbox", "an inbox item")
	writeNote(t, filepath.Join(root, "_archive"), "archive-note.md", "archive", "an archived item")

	out, _, err := runPark(t, root, "list", "--json", "--category", "archive")
	if err != nil {
		t.Fatalf("list --json --category error = %v", err)
	}

	var env struct {
		SchemaVersion int `json:"schema_version"`
		Items         []struct {
			Filename string `json:"filename"`
			Category string `json:"category"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("parse list json = %v\n%s", err, out)
	}
	if env.SchemaVersion != schema.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", env.SchemaVersion, schema.SchemaVersion)
	}
	if len(env.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(env.Items))
	}
	if env.Items[0].Filename != "archive-note.md" || env.Items[0].Category != "archive" {
		t.Errorf("item = %+v, want archive-note.md in archive", env.Items[0])
	}
}

func TestListAlias(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "alias-note.md", "inbox", "an item")

	out, _, err := runPark(t, root, "ls")
	if err != nil {
		t.Fatalf("ls error = %v", err)
	}
	if !strings.Contains(out, "alias-note.md") {
		t.Errorf("ls output missing note:\n%s", out)
	}
}

func TestCheckCleanStore(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "solo-note.md", "inbox", "an item")

	out, _, err := runPark(t, root, "check")
	if err != nil {
		t.Fatalf("check error = %v", err)
	}
	if out != "" {
		t.Errorf("check output = %q, want empty", out)
	}
}

func TestCheckReportsDuplicates(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "dup-note.md", "inbox", "inbox copy")
	writeNote(t, filepath.Join(root, "_projects"), "dup-note.md", "projects", "projects copy")

	out, _, err := runPark(t, root, "check")
	if err == nil {
		t.Fatal("expected check to fail on duplicate note names")
	}
	if got := exitCode(t, err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
	want := "duplicate: dup-note.md\n" +
		"  inbox     " + filepath.Join(root, "_inbox", "dup-note.md") + "\n" +
		"  projects  " + filepath.Join(root, "_projects", "dup-note.md") + "\n" +
		"  rename or remove all but one copy\n"
	if out != want {
		t.Errorf("check output = %q, want %q", out, want)
	}
}

func TestNewRejectsDuplicateAcrossCategories(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	if _, _, err := runPark(t, root, "new", "shared-name", "-s", "first", "-src", "test"); err != nil {
		t.Fatalf("first new error = %v", err)
	}

	_, _, err := runPark(t, root, "new", "shared-name", "-s", "second", "-src", "test", "-c", "projects")
	if err == nil {
		t.Fatal("expected error for a name already parked in another category")
	}
	if got := exitCode(t, err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
	if !strings.Contains(err.Error(), "note already exists in inbox") {
		t.Errorf("error = %q, want it to name the category holding the note", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "_projects", "shared-name.md")); !os.IsNotExist(statErr) {
		t.Errorf("projects note exists despite the error: %v", statErr)
	}
}

func TestShowBareStem(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "stem-note.md", "inbox", "a stem note")

	out, _, err := runPark(t, root, "show", "stem-note")
	if err != nil {
		t.Fatalf("show stem error = %v", err)
	}
	assertPlainShow(t, out, "a stem note")
}

func TestShowAmbiguousStem(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "dup-note.md", "inbox", "inbox copy")
	writeNote(t, filepath.Join(root, "_projects"), "dup-note.md", "projects", "projects copy")

	_, _, err := runPark(t, root, "show", "dup-note")
	if err == nil {
		t.Fatal("expected error for an ambiguous stem")
	}
	if got := exitCode(t, err); got != 1 {
		t.Errorf("exit code = %d, want 1", got)
	}
	for _, want := range []string{"ambiguous", filepath.Join("_inbox", "dup-note.md"), filepath.Join("_projects", "dup-note.md"), "park check"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %q", err, want)
		}
	}
}

func TestReclassifyBareStem(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}
	writeNote(t, filepath.Join(root, "_inbox"), "movable.md", "inbox", "an inbox item")

	if _, _, err := runPark(t, root, "reclassify", "movable", "-c", "areas"); err != nil {
		t.Fatalf("reclassify stem error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "_areas", "movable.md")); err != nil {
		t.Errorf("note was not moved: %v", err)
	}
}

func TestShowMissingBareStemNamesArgument(t *testing.T) {
	root := t.TempDir()
	if _, _, err := runPark(t, root, "init"); err != nil {
		t.Fatalf("init error = %v", err)
	}

	_, _, err := runPark(t, root, "show", "ghost")
	if err == nil {
		t.Fatal("expected error for a missing stem")
	}
	if !strings.Contains(err.Error(), `"ghost"`) {
		t.Errorf("error = %q, want it to name the argument", err)
	}
	if !strings.Contains(err.Error(), "not found in any category folder") {
		t.Errorf("error = %q, want it to name the search scope", err)
	}
}
