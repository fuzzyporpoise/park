// Package render handles glamour-based rendering of parked notes.
package render

import (
	"fmt"
	"io"
	"sync"

	"charm.land/glamour/v2"

	"go.fuzzyporpoise.dev/park/internal/note"
)

const (
	richStyle  = "dark"
	plainStyle = "notty"
	wordWrap   = 100
)

var (
	renderersMu sync.Mutex
	renderers   = map[string]*glamour.TermRenderer{}
	rendererErr error
)

func termRenderer(style string) (*glamour.TermRenderer, error) {
	renderersMu.Lock()
	defer renderersMu.Unlock()

	if rendererErr != nil {
		return nil, rendererErr
	}
	if r, ok := renderers[style]; ok {
		return r, nil
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(style),
		glamour.WithWordWrap(wordWrap),
	)
	if err != nil {
		rendererErr = err
		return nil, err
	}
	renderers[style] = r
	return r, nil
}

// noteHeader builds the frontmatter summary line rendered above the body.
// plain selects the decoration-free variant used by notty output.
func noteHeader(n note.Note, plain bool) string {
	if plain {
		return fmt.Sprintf(
			"category: %s   created: %s   source: %s\n\n> %s\n\n---\n\n",
			n.Category, n.Created, n.Source, n.Synopsis,
		)
	}
	return fmt.Sprintf(
		"**category:** %s &nbsp;&nbsp; **created:** %s &nbsp;&nbsp; **source:** %s\n\n> %s\n\n---\n\n",
		n.Category, n.Created, n.Source, n.Synopsis,
	)
}

// ShowFile renders a parked note's frontmatter summary + body to w via
// glamour: the "look deeper" step after the synopsis in the list view
// earned a second look. When plain is true it uses glamour's notty style,
// producing readable text with no ANSI escapes for pipes and redirects.
func ShowFile(path string, w io.Writer, plain bool) error {
	n, err := note.Parse(path)
	if err != nil {
		return fmt.Errorf("show %q: %w", path, err)
	}

	style := richStyle
	if plain {
		style = plainStyle
	}

	renderer, err := termRenderer(style)
	if err != nil {
		return fmt.Errorf("create glamour renderer: %w", err)
	}

	out, err := renderer.Render(noteHeader(n, plain) + n.Body)
	if err != nil {
		return fmt.Errorf("render %q: %w", path, err)
	}
	if _, err := fmt.Fprint(w, out); err != nil {
		return fmt.Errorf("write rendered output: %w", err)
	}
	return nil
}
