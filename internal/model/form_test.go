package model

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/note"
	"go.fuzzyporpoise.dev/park/internal/store"
)

func fieldIndex(m NoteFormModel, f formField) int {
	for i, ff := range m.fields {
		if ff == f {
			return i
		}
	}
	return -1
}

func TestNewNoteFormModel(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	seed := note.Draft{
		Filename: "filename",
		Body:     "body",
		Metadata: note.Metadata{Synopsis: "synopsis", Source: "source", Category: "inbox"},
	}
	m, err := NewNoteFormModel(cfg, seed)
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}

	if m.inputs[0].Value() != "filename" {
		t.Errorf("filename = %q, want filename", m.inputs[0].Value())
	}
	if m.inputs[1].Value() != "synopsis" {
		t.Errorf("synopsis = %q, want synopsis", m.inputs[1].Value())
	}
	if m.inputs[2].Value() != "source" {
		t.Errorf("source = %q, want source", m.inputs[2].Value())
	}
	if m.categoryName() != "inbox" {
		t.Errorf("category = %q, want inbox", m.categoryName())
	}
	if m.bodyInput.Value() != "body" {
		t.Errorf("body = %q, want body", m.bodyInput.Value())
	}
}

func TestNewNoteFormModelUnknownCategory(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	_, err := NewNoteFormModel(cfg, note.Draft{Metadata: note.Metadata{Category: "nope"}})
	if err == nil {
		t.Fatal("expected error for unknown category")
	}
}

func TestNoteFormModelSubmission(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := store.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	seed := note.Draft{
		Filename: "Form Note",
		Body:     "# Form Note\n",
		Metadata: note.Metadata{Synopsis: "form synopsis", Source: "test", Category: "inbox"},
	}
	m, err := NewNoteFormModel(cfg, seed)
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}

	updated, cmd := m.Update(submitMsg{})
	final, ok := updated.(NoteFormModel)
	if !ok {
		t.Fatalf("unexpected model type")
	}
	if final.err != nil {
		t.Fatalf("submit error = %v", final.err)
	}
	if cmd == nil {
		t.Fatal("expected create command")
	}

	msg := cmd()
	created, ok := msg.(createResult)
	if !ok {
		t.Fatalf("expected createResult, got %T", msg)
	}
	if created.err != nil {
		t.Fatalf("unexpected create error: %v", created.err)
	}
	if created.path == "" {
		t.Fatal("expected non-empty path")
	}
}

func TestNoteFormModelRequiresSource(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	if _, _, err := store.Init(cfg); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	seed := note.Draft{
		Filename: "Filename",
		Metadata: note.Metadata{Synopsis: "Synopsis", Category: "inbox"},
	}
	m, err := NewNoteFormModel(cfg, seed)
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}

	updated, cmd := m.Update(submitMsg{})
	final, ok := updated.(NoteFormModel)
	if !ok {
		t.Fatalf("unexpected model type")
	}
	if cmd == nil {
		t.Fatal("expected validation command")
	}

	msg := cmd()
	res, ok := msg.(createResult)
	if !ok {
		t.Fatalf("expected createResult, got %T", msg)
	}
	if res.err == nil {
		t.Fatal("expected error for missing source")
	}
	final.err = res.err
	if final.err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestNoteFormModelView(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	m, err := NewNoteFormModel(cfg, note.Draft{Metadata: note.Metadata{Category: "inbox"}})
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}

	_ = m.View()
}

func TestNoteFormModelCategoryNavigation(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	m, err := NewNoteFormModel(cfg, note.Draft{Metadata: note.Metadata{Category: "inbox"}})
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}

	m.focusIndex = fieldIndex(m, fieldCategory)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	final, ok := updated.(NoteFormModel)
	if !ok {
		t.Fatalf("unexpected model type")
	}
	if final.categoryName() != "projects" {
		t.Errorf("category after right = %q, want projects", final.categoryName())
	}
}

func TestNoteFormModelCancelKeys(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	m, err := NewNoteFormModel(cfg, note.Draft{Metadata: note.Metadata{Category: "inbox"}})
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}

	cancelKeys := []tea.KeyPressMsg{
		{Code: tea.KeyEsc},
		{Code: 'c', Mod: tea.ModCtrl},
	}
	for _, k := range cancelKeys {
		updated, cmd := m.Update(k)
		if _, ok := updated.(NoteFormModel); !ok {
			t.Fatalf("unexpected model type for %v", k)
		}
		if cmd == nil {
			t.Fatalf("expected quit command for %v", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("expected QuitMsg for %v, got %T", k, cmd())
		}
	}
}

func TestNoteFormModelCategoryKeyBindings(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())

	tests := []struct {
		name string
		key  tea.KeyPressMsg
		want string
	}{
		{name: "left arrow", key: tea.KeyPressMsg{Code: tea.KeyLeft}, want: "archive"},
		{name: "h", key: keyPress('h'), want: "archive"},
		{name: "right arrow", key: tea.KeyPressMsg{Code: tea.KeyRight}, want: "projects"},
		{name: "l", key: keyPress('l'), want: "projects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := NewNoteFormModel(cfg, note.Draft{Metadata: note.Metadata{Category: "inbox"}})
			if err != nil {
				t.Fatalf("NewNoteFormModel() error = %v", err)
			}
			m.focusIndex = fieldIndex(m, fieldCategory)
			updated, _ := m.Update(tt.key)
			final, ok := updated.(NoteFormModel)
			if !ok {
				t.Fatalf("unexpected model type")
			}
			if final.categoryName() != tt.want {
				t.Errorf("category after %s = %q, want %q", tt.name, final.categoryName(), tt.want)
			}
		})
	}
}

func TestNoteFormModelBodyCursorStartsAtTop(t *testing.T) {
	cfg := config.DefaultConfig(t.TempDir())
	body := strings.Repeat("line\n", 20)
	m, err := NewNoteFormModel(cfg, note.Draft{Body: body, Metadata: note.Metadata{Category: "inbox"}})
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}

	m.focusIndex = fieldIndex(m, fieldBody)
	m, _ = m.updateFocus()

	if m.bodyInput.ScrollYOffset() != 0 {
		t.Errorf("body scroll offset = %d, want 0", m.bodyInput.ScrollYOffset())
	}
}

func TestNoteFormModelFilePreview(t *testing.T) {
	tmp := t.TempDir()
	cfg := config.DefaultConfig(tmp)
	path := tmp + "/draft.md"
	if err := note.Write(path, note.Note{Body: "# Draft\n\ncontent\n", Metadata: note.Metadata{Category: "inbox"}}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	m, err := NewNoteFormModel(cfg, note.Draft{FromFile: path, Metadata: note.Metadata{Category: "inbox"}})
	if err != nil {
		t.Fatalf("NewNoteFormModel() error = %v", err)
	}
	if m.hasBodyField() {
		t.Fatal("expected body field to be hidden when from-file is set")
	}

	cmd := loadPreviewCmd(path)
	msg := cmd()
	previewMsg, ok := msg.(filePreviewLoadedMsg)
	if !ok {
		t.Fatalf("expected filePreviewLoadedMsg, got %T", msg)
	}
	m.filePreview = previewMsg.preview
	if m.filePreview == "" {
		t.Fatal("expected file preview to be populated")
	}
}
