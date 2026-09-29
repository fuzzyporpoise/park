package model

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/store"
	"go.fuzzyporpoise.dev/park/schema"
)

// listItem adapts an Item to bubbles/list's list.Item interface.
type listItem struct {
	item store.Item
}

func (i listItem) Title() string {
	created, _ := time.Parse(schema.DateFormat, i.item.Created)
	if created.IsZero() {
		created = i.item.ModTime
	}
	return fmt.Sprintf("%s  ·  %s", i.item.Filename, humanAge(created))
}
func (i listItem) Description() string { return i.item.Synopsis }
func (i listItem) FilterValue() string {
	return i.item.Filename + " " + i.item.Synopsis
}

// styledDelegate returns a list.DefaultDelegate with the themed foreground colors.
func (s *styles) styledDelegate() list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.Styles.NormalTitle = d.Styles.NormalTitle.Foreground(s.listNormalTitle.GetForeground())
	d.Styles.NormalDesc = d.Styles.NormalDesc.Foreground(s.listNormalDesc.GetForeground())
	d.Styles.SelectedTitle = d.Styles.SelectedTitle.Foreground(s.listSelectedTitle.GetForeground())
	d.Styles.SelectedDesc = d.Styles.SelectedDesc.Foreground(s.listSelectedDesc.GetForeground())
	d.Styles.DimmedTitle = d.Styles.DimmedTitle.Foreground(s.listDimmedTitle.GetForeground())
	d.Styles.DimmedDesc = d.Styles.DimmedDesc.Foreground(s.listDimmedDesc.GetForeground())
	return d
}

func humanAge(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// categoryBinding pairs a category name with the key binding that both
// renders it in help and matches it in Update, so the two can never drift.
type categoryBinding struct {
	name    string
	binding key.Binding
}

// keyMap defines the key bindings for the park TUI.
type keyMap struct {
	View             key.Binding
	CycleNext        key.Binding
	CyclePrev        key.Binding
	CategoryBindings []categoryBinding
	Help             key.Binding
	Quit             key.Binding
}

// Primary returns navigation shortcuts shown in the collapsed help bar.
func (k keyMap) Primary() []key.Binding {
	return []key.Binding{k.View, k.CycleNext, k.CyclePrev, k.Help, k.Quit}
}

// Categories returns reclassify/category action shortcuts shown only in full help.
func (k keyMap) Categories() []key.Binding {
	bindings := make([]key.Binding, len(k.CategoryBindings))
	for i, cb := range k.CategoryBindings {
		bindings[i] = cb.binding
	}
	return bindings
}

func (k keyMap) ShortHelp() []key.Binding {
	return k.Primary()
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		k.Primary(),
		k.Categories(),
	}
}

var keys = keyMap{
	View: key.NewBinding(
		key.WithKeys("enter", "v"),
		key.WithHelp("enter/v", "view"),
	),
	CycleNext: key.NewBinding(
		key.WithKeys("right", "l", "ctrl+n"),
		key.WithHelp("→/l", "next category"),
	),
	CyclePrev: key.NewBinding(
		key.WithKeys("left", "h", "ctrl+p"),
		key.WithHelp("←/h", "prev category"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q/ctrl+c", "quit"),
	),
}

// newKeyMap builds a keyMap from the static bindings plus one reclassify binding per
// configured category that has a non-empty Key.
func newKeyMap(cfg *config.Config) keyMap {
	km := keys
	km.CategoryBindings = make([]categoryBinding, 0, len(cfg.Categories))
	for _, cl := range cfg.Categories {
		if cl.Key == "" {
			continue
		}
		km.CategoryBindings = append(km.CategoryBindings, categoryBinding{
			name: cl.Name,
			binding: key.NewBinding(
				key.WithKeys(cl.Key),
				key.WithHelp(cl.Key, cl.Name),
			),
		})
	}
	return km
}

// AssistModel is the bubbletea model for the park TUI with tabbed category navigation.
type AssistModel struct {
	list        list.Model
	categoryIdx int
	cfg         *config.Config
	err         error
	ViewFile    string
	styles      *styles
	help        help.Model
	keys        keyMap
	width       int
}

func NewAssistModel(cfg *config.Config) (AssistModel, error) {
	idx := -1
	for i, cl := range cfg.Categories {
		if cl.Name == cfg.DefaultCategory {
			idx = i
			break
		}
	}
	if idx < 0 {
		return AssistModel{}, fmt.Errorf("default category %q not found", cfg.DefaultCategory)
	}

	s := newStyles()
	delegate := s.styledDelegate()

	m := AssistModel{
		categoryIdx: idx,
		cfg:         cfg,
		keys:        newKeyMap(cfg),
		help:        help.New(),
		styles:      s,
		list:        list.New(nil, delegate, maxWidth, defaultHeight),
		width:       maxWidth,
	}
	m.list.SetFilteringEnabled(true)
	m.list.SetShowHelp(false)
	m.list.SetShowTitle(false)
	m.list.SetShowStatusBar(false)
	return m, nil
}

func (m AssistModel) switchCategory(delta int) AssistModel {
	m.categoryIdx = cycleIndex(m.categoryIdx, delta, len(m.cfg.Categories))
	return m
}

// loadResult carries the outcome of an async category load back to Update:
// either the loaded items (err == nil) or a load error.
type loadResult struct {
	categoryName string
	items        []list.Item
	err          error
}

// reclassifyResult carries the outcome of an async reclassify back to
// Update: either the moved item (err == nil) or a reclassify error.
type reclassifyResult struct {
	item listItem
	err  error
}

func (m AssistModel) loadItemsCmd() tea.Cmd {
	categoryName := m.cfg.Categories[m.categoryIdx].Name
	cfg := m.cfg
	return func() tea.Msg {
		items, err := store.Scan(cfg, categoryName)
		if err != nil {
			return loadResult{categoryName: categoryName, err: err}
		}
		listItems := make([]list.Item, len(items))
		for i, it := range items {
			listItems[i] = listItem{item: it}
		}
		return loadResult{categoryName: categoryName, items: listItems}
	}
}

func (m AssistModel) reclassifyCmd(targetCategory string) tea.Cmd {
	it, ok := m.list.SelectedItem().(listItem)
	if !ok {
		return func() tea.Msg {
			return reclassifyResult{err: fmt.Errorf("no item selected")}
		}
	}
	filename := it.item.Filename
	cfg := m.cfg
	return func() tea.Msg {
		if err := store.Reclassify(cfg, filename, targetCategory); err != nil {
			return reclassifyResult{err: err}
		}
		return reclassifyResult{item: it}
	}
}

func (m AssistModel) removeItem(target listItem) AssistModel {
	for i, it := range m.list.Items() {
		if li, ok := it.(listItem); ok && li.item.Path == target.item.Path {
			m.list.RemoveItem(i)
			return m
		}
	}
	return m
}

func (m AssistModel) Init() tea.Cmd { return m.loadItemsCmd() }

// keeps assist and new TUI screens approx same size
const maxListHeight = 28

func (m AssistModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = min(maxWidth, msg.Width)
		m.help.SetWidth(m.width)
		m.list.SetSize(m.width, min(max(5, msg.Height-6), maxListHeight))

	case loadResult:
		currentCategory := m.cfg.Categories[m.categoryIdx].Name
		if msg.categoryName != currentCategory {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.list.SetItems(msg.items)
		m.err = nil
		return m, nil

	case reclassifyResult:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m = m.removeItem(msg.item)
		m.err = nil
		return m, nil

	case tea.KeyPressMsg:
		m.err = nil
		if m.list.SettingFilter() {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			break
		}
		switch {
		case key.Matches(msg, m.keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keys.View):
			if it, ok := m.list.SelectedItem().(listItem); ok {
				m.ViewFile = it.item.Path
				return m, tea.Quit
			}
		case key.Matches(msg, m.keys.CycleNext):
			m = m.switchCategory(+1)
			return m, m.loadItemsCmd()
		case key.Matches(msg, m.keys.CyclePrev):
			m = m.switchCategory(-1)
			return m, m.loadItemsCmd()
		case key.Matches(msg, m.keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			return m, nil
		default:
			for _, cb := range m.keys.CategoryBindings {
				if !key.Matches(msg, cb.binding) {
					continue
				}
				currentCategory := m.cfg.Categories[m.categoryIdx].Name
				if cb.name == currentCategory {
					m.err = fmt.Errorf("already in %s", cb.name)
					return m, nil
				}
				return m, m.reclassifyCmd(cb.name)
			}
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m AssistModel) View() tea.View {
	if m.styles == nil {
		return tea.NewView("")
	}

	doc := strings.Builder{}
	s := m.styles

	header := s.renderTabs(m.cfg.Categories, m.categoryIdx, m.width, false)

	doc.WriteString(header)
	doc.WriteString("\n")
	doc.WriteString(m.list.View())

	var footer string
	{
		if m.help.ShowAll {
			helpTitle := s.highlight.Render("set item category")
			footer += s.window.Width(m.width).Render(helpTitle)
		}
		footer += s.window.Width(m.width).Render(m.help.View(m.keys))
		if m.err != nil {
			errorText := s.errorText.Render(m.err.Error())
			footer += s.window.Width(m.width).Render(errorText)
		}
	}

	doc.WriteString(footer)

	v := tea.NewView(s.doc.Render(doc.String()))
	v.AltScreen = true
	return v
}
