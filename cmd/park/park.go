package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"go.fuzzyporpoise.dev/park/schema"

	tea "charm.land/bubbletea/v2"
	"github.com/urfave/cli/v3"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/model"
	"go.fuzzyporpoise.dev/park/internal/note"
	"go.fuzzyporpoise.dev/park/internal/render"
	"go.fuzzyporpoise.dev/park/internal/store"
)

// isTerminal reports whether the given file descriptor is connected to an
// interactive terminal. It is used for both stdin (see stdinIsTTY) and stderr
// (for styled error output).
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// stdinIsTTY reports whether os.Stdin is connected to an interactive terminal.
func stdinIsTTY() bool {
	return isTerminal(os.Stdin)
}

// writerIsTTY reports whether w is connected to an interactive terminal. A
// writer that is not an *os.File (a buffer, a pipe) is treated as
// non-terminal so callers fall back to plain output.
func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isTerminal(f)
}

// draftFromCmd builds a note.Draft from the CLI flags and positional args.
func draftFromCmd(cmd *cli.Command) note.Draft {
	d := note.Draft{
		Filename: cmd.String("filename"),
		Metadata: note.Metadata{
			Synopsis: cmd.String("synopsis"),
			Source:   cmd.String("source"),
			Category: cmd.String("category"),
		},
		FromFile: cmd.String("from-file"),
	}
	if d.Filename == "" {
		d.Filename = cmd.Args().First()
	}
	return d
}

// printParked writes the standard "parked: <path>" confirmation.
func printParked(w io.Writer, path string) error {
	if _, err := fmt.Fprintln(w, "parked:", path); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}

// addPark parks a new note. It reads CLI input and delegates the creation
// decision to internal/note. If the input is incomplete, it opens the
// bubbletea form for the missing metadata.
func addPark(cfg *config.Config, cmd *cli.Command, w io.Writer) error {
	d := draftFromCmd(cmd)

	if !stdinIsTTY() {
		data, err := io.ReadAll(cmd.Reader)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		d.Body = string(data)
	}

	outcome, err := note.Add(cfg, d)
	if err != nil {
		return err
	}
	if outcome.Path != "" {
		return printParked(w, outcome.Path)
	}
	return runNoteForm(cfg, w, outcome.Form)
}

func runNoteForm(cfg *config.Config, w io.Writer, seed *note.Draft) error {
	m, err := model.NewNoteFormModel(cfg, *seed)
	if err != nil {
		return err
	}

	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}

	final, ok := result.(model.NoteFormModel)
	if !ok {
		return fmt.Errorf("unexpected model type from form")
	}
	res := final.Result()
	if res.Err != nil {
		return res.Err
	}
	if res.Path != "" {
		return printParked(w, res.Path)
	}
	return nil
}

// schemaPark prints the canonical frontmatter schema, either as JSON or as
// a short human-readable summary.
func schemaPark(asJSON bool, w io.Writer) error {
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(schema.Describe()); err != nil {
			return fmt.Errorf("write schema output: %w", err)
		}
		return nil
	}

	s := schema.Describe()
	var b strings.Builder
	fmt.Fprintf(&b, "park frontmatter schema (version %d)\n\n", s.SchemaVersion)
	fmt.Fprintln(&b, "Fields:")
	for _, f := range s.Fields {
		extra := ""
		switch f.Kind {
		case "enum":
			extra = "values: " + strings.Join(f.Values, ", ")
		case "date":
			extra = "format: " + f.Format
			if f.Auto {
				extra += " (auto-stamped)"
			}
		}
		if extra != "" {
			fmt.Fprintf(&b, "  %-8s  required %-5s  %s\n", f.Name, f.Kind, extra)
		} else {
			fmt.Fprintf(&b, "  %-8s  required %-5s\n", f.Name, f.Kind)
		}
	}
	fmt.Fprintf(&b, "\nDate format: %s\n", s.DateFormat)
	fmt.Fprintf(&b, "Write template:\n%s", s.WriteTemplate)
	if _, err := fmt.Fprint(w, b.String()); err != nil {
		return fmt.Errorf("write schema output: %w", err)
	}
	return nil
}

// listPark lists parked notes grouped by category, either as plain text or as
// the versioned JSON envelope.
func listPark(cfg *config.Config, opts store.ListOptions, asJSON bool, w io.Writer) error {
	groups, err := store.List(cfg, opts)
	if err != nil {
		return err
	}
	if asJSON {
		return store.WriteListJSON(w, groups)
	}
	return store.FormatList(w, groups)
}

func assistPark(cfg *config.Config, w io.Writer) error {
	m, err := model.NewAssistModel(cfg)
	if err != nil {
		return err
	}

	result, err := tea.NewProgram(m).Run()
	if err != nil {
		return err
	}

	final, ok := result.(model.AssistModel)
	if !ok {
		return fmt.Errorf("unexpected model type from assist")
	}
	if final.ViewFile != "" {
		if err := render.ShowFile(final.ViewFile, w, !writerIsTTY(w)); err != nil {
			return err
		}
	}
	return nil
}
