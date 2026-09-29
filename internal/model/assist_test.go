package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/note"
	"go.fuzzyporpoise.dev/park/internal/store"
)

func newTestAssistModel(t *testing.T, items ...store.Item) AssistModel {
	t.Helper()
	cfg := config.DefaultConfig(t.TempDir())
	if _, _, err := store.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	m, err := NewAssistModel(cfg)
	if err != nil {
		t.Fatalf("NewAssistModel() error = %v", err)
	}

	if len(items) > 0 {
		updated, _ := m.Update(makeLoadResult(cfg.DefaultCategory, items...))
		m = updated.(AssistModel)
	}
	return m
}

func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func makeLoadResult(category string, items ...store.Item) loadResult {
	listItems := make([]list.Item, len(items))
	for i, it := range items {
		listItems[i] = listItem{item: it}
	}
	return loadResult{categoryName: category, items: listItems}
}

func TestNewAssistModelDefaultCategory(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	m, err := NewAssistModel(cfg)
	if err != nil {
		t.Fatalf("NewAssistModel() error = %v", err)
	}
	if m.cfg.Categories[m.categoryIdx].Name != cfg.DefaultCategory {
		t.Errorf("default category = %q, want %q", m.cfg.Categories[m.categoryIdx].Name, cfg.DefaultCategory)
	}
}

func TestAssistModelLoadResult(t *testing.T) {
	items := []store.Item{
		{
			Metadata: note.Metadata{Category: "inbox", Created: "2026-08-09", Source: "test", Synopsis: "first"},
			Path:     "/tmp/inbox/first.md",
			Filename: "first.md",
			ModTime:  time.Now(),
		},
		{
			Metadata: note.Metadata{Category: "inbox", Created: "2026-08-09", Source: "test", Synopsis: "second"},
			Path:     "/tmp/inbox/second.md",
			Filename: "second.md",
			ModTime:  time.Now(),
		},
	}
	m := newTestAssistModel(t, items...)

	if len(m.list.Items()) != 2 {
		t.Errorf("list items = %d, want 2", len(m.list.Items()))
	}
}

func TestAssistModelStaleLoadResultIgnored(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	if _, _, err := store.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	m, err := NewAssistModel(cfg)
	if err != nil {
		t.Fatalf("NewAssistModel() error = %v", err)
	}

	// Switch to projects, then deliver an inbox load result.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m = updated.(AssistModel)

	updated, _ = m.Update(makeLoadResult("inbox", store.Item{Path: "/tmp/inbox/old.md", Filename: "old.md"}))
	m = updated.(AssistModel)

	if len(m.list.Items()) != 0 {
		t.Errorf("stale result items = %d, want 0", len(m.list.Items()))
	}
}

func TestAssistModelCycleCategory(t *testing.T) {
	m := newTestAssistModel(t)
	startIdx := m.categoryIdx

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	final := updated.(AssistModel)
	if final.categoryIdx == startIdx {
		t.Error("category index did not advance")
	}
	if cmd == nil {
		t.Fatal("expected load command after category change")
	}
}

func TestAssistModelQuit(t *testing.T) {
	m := newTestAssistModel(t)
	updated, cmd := m.Update(keyPress('q'))
	_ = updated.(AssistModel)
	if cmd == nil {
		t.Fatal("expected quit command")
	}
}

func TestAssistModelToggleHelp(t *testing.T) {
	m := newTestAssistModel(t)
	updated, _ := m.Update(keyPress('?'))
	final := updated.(AssistModel)
	if !final.help.ShowAll {
		t.Error("help.ShowAll = false after toggling help")
	}
}

func TestAssistModelReclassifySameCategory(t *testing.T) {
	items := []store.Item{
		{
			Metadata: note.Metadata{Category: "inbox", Created: "2026-08-09", Source: "test", Synopsis: "stay"},
			Path:     "/tmp/inbox/stay.md",
			Filename: "stay.md",
			ModTime:  time.Now(),
		},
	}
	m := newTestAssistModel(t, items...)

	updated, cmd := m.Update(keyPress('i'))
	final := updated.(AssistModel)
	if cmd != nil {
		t.Fatal("expected no command for same-category reclassify")
	}
	if final.err == nil {
		t.Fatal("expected error for same-category reclassify")
	}
}

func TestAssistModelReclassifySuccess(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := store.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	path, err := note.Create(cfg, note.Draft{
		Filename: "Move Me",
		Metadata: note.Metadata{Synopsis: "move", Source: "test", Category: "inbox"},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	m, err := NewAssistModel(cfg)
	if err != nil {
		t.Fatalf("NewAssistModel() error = %v", err)
	}

	items, err := store.Scan(cfg, "inbox")
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	updated, _ := m.Update(makeLoadResult("inbox", items...))
	m = updated.(AssistModel)

	if len(m.list.Items()) != 1 {
		t.Fatalf("list items = %d, want 1", len(m.list.Items()))
	}

	updated, cmd := m.Update(keyPress('p'))
	m = updated.(AssistModel)
	if cmd == nil {
		t.Fatal("expected reclassify command")
	}

	msg := cmd()
	res, ok := msg.(reclassifyResult)
	if !ok {
		t.Fatalf("expected reclassifyResult, got %T", msg)
	}
	if res.err != nil {
		t.Fatalf("reclassify command error = %v", res.err)
	}

	updated, _ = m.Update(res)
	final := updated.(AssistModel)
	if final.err != nil {
		t.Errorf("model err = %v", final.err)
	}
	if len(final.list.Items()) != 0 {
		t.Errorf("list items after reclassify = %d, want 0", len(final.list.Items()))
	}

	projectsPath := filepath.Join(tmp, "_projects", filepath.Base(path))
	if _, err := os.Stat(projectsPath); err != nil {
		t.Errorf("file not in projects: %v", err)
	}
}
