// Package cmd defines the park CLI command tree and wiring.
package cmd

import (
	"context"
	"fmt"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/urfave/cli/v3"
	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/render"
	"go.fuzzyporpoise.dev/park/internal/store"
	"go.fuzzyporpoise.dev/park/internal/theme"
)

var (
	version   = "internal"
	buildTime string
)

// SetVersion sets the application version string used by the CLI.
func SetVersion(v string) {
	version = v
}

// SetBuildTime sets the build timestamp for version display.
func SetBuildTime(bt string) {
	buildTime = bt
}

func buildVersion() string {
	v := version
	if buildTime != "" {
		v += " (" + buildTime + ")"
	}
	return v
}

func Main() {
	if err := newCommand().Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, styledError(err))
		os.Exit(1)
	}
}

// newCommand builds the park CLI command tree. It is extracted from Main so
// tests can drive dispatch without touching os.Args or process exit.
func newCommand() *cli.Command {
	var cfg *config.Config

	var (
		parkRoot, parkConfig string
		reclassifyCategory   string
		newCategory          string
		schemaJSON           bool
		listJSON             bool
		listAll              bool
		showPlain            bool
	)

	defaultRoot := os.Getenv("PARK_ROOT")
	if defaultRoot == "" {
		defaultRoot = config.DefaultRootPath()
	}

	return &cli.Command{
		Name:                  "park",
		Usage:                 "IPAA: a parking lot for markdown notes (Inbox/Projects/Areas/Archive)",
		Version:               buildVersion(),
		EnableShellCompletion: true,
		HideVersion:           false,
		Reader:                os.Stdin,
		Writer:                os.Stdout,
		ErrWriter:             os.Stderr,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "park-root",
				Destination: &parkRoot,
				Value:       defaultRoot,
				Sources:     cli.EnvVars("PARK_ROOT"),
				Usage:       "root directory for parked notes",
			},
			&cli.StringFlag{
				Name:        "park-config",
				Destination: &parkConfig,
				Value:       config.DefaultConfigPathFor(defaultRoot),
				Sources:     cli.EnvVars("PARK_CONFIG"),
				Usage:       "path to the config file",
			},
		},
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			cfg = &config.Config{}
			if err := cfg.LoadConfig(parkRoot, parkConfig); err != nil {
				return ctx, styledExit(err, 1)
			}
			return ctx, nil
		},
		Commands: []*cli.Command{
			{
				Name:  "assist",
				Usage: "browse parked files and/or edit categories",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := assistPark(cfg, cmd.Root().Writer); err != nil {
						return styledExit(err, 1)
					}
					return nil
				},
			},
			{
				Name:  "config",
				Usage: "print the loaded configuration",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					out, err := cfg.Dump()
					if err != nil {
						return styledExit(err, 1)
					}
					if _, err := fmt.Fprint(cmd.Root().Writer, out); err != nil {
						return fmt.Errorf("write config output: %w", err)
					}
					return nil
				},
			},
			{
				Name:  "check",
				Usage: "verify that category folders exist (useful for automation)",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					missing, err := store.Check(cfg)
					if err != nil {
						return styledExit(err, 1)
					}
					if len(missing) > 0 {
						for _, p := range missing {
							if _, err := fmt.Fprintf(cmd.Root().Writer, "missing: %s\n", p); err != nil {
								return fmt.Errorf("write check output: %w", err)
							}
						}
						return styledExit(fmt.Errorf("%d category folder(s) missing", len(missing)), 1)
					}
					return nil
				},
			},
			{
				Name:  "init",
				Usage: "create the category folders (idempotent)",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					created, existed, err := store.Init(cfg)
					if err != nil {
						return styledExit(err, 1)
					}
					msg := store.FormatInitResult(created, existed)
					if _, err := fmt.Fprintln(cmd.Root().Writer, msg); err != nil {
						return fmt.Errorf("write init output: %w", err)
					}
					return nil
				},
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "list parked notes grouped by category",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:        "json",
						Destination: &listJSON,
						Usage:       "output machine-readable JSON",
					},
					&cli.BoolFlag{
						Name:        "all",
						Destination: &listAll,
						Usage:       "include categories excluded by config",
					},
					&cli.StringSliceFlag{
						Name:    "category",
						Aliases: []string{"c"},
						Usage:   "only list these categories (repeatable; overrides exclusion)",
					},
				},
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					for _, name := range cmd.StringSlice("category") {
						if !cfg.HasCategory(name) {
							return ctx, styledExit(cfg.UnknownCategoryError(name), 2)
						}
					}
					return ctx, nil
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					opts := store.ListOptions{
						IncludeExcluded: listAll,
						Categories:      cmd.StringSlice("category"),
					}
					if err := listPark(cfg, opts, listJSON, cmd.Root().Writer); err != nil {
						return styledExit(err, 1)
					}
					return nil
				},
			},
			{
				Name:      "new",
				Aliases:   []string{"add"},
				Usage:     "park a new note",
				ArgsUsage: "[filename]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "synopsis",
						Aliases: []string{"s"},
						Usage:   "one-line synopsis (what it is / why it matters)",
					},
					&cli.StringFlag{
						Name:    "source",
						Aliases: []string{"src"},
						Usage:   "where this came from (repo name, chat, etc.)",
					},
					&cli.StringFlag{
						Name:  "filename",
						Usage: "note filename (used for the file slug)",
					},
					&cli.StringFlag{
						Name:        "category",
						Aliases:     []string{"c"},
						Destination: &newCategory,
						Usage:       "category to park directly into if not the default",
					},
					&cli.StringFlag{
						Name:    "from-file",
						Aliases: []string{"f"},
						Usage:   "existing markdown file to move into the park",
					},
				},
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					if newCategory != "" {
						if !cfg.HasCategory(newCategory) {
							return ctx, styledExit(cfg.UnknownCategoryError(newCategory), 1)
						}
					}
					return ctx, nil
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := addPark(cfg, cmd, cmd.Root().Writer); err != nil {
						return styledExit(err, 1)
					}
					return nil
				},
			},
			{
				Name:      "reclassify",
				Aliases:   []string{"recat"},
				Usage:     "reclassify a note",
				ArgsUsage: "<file>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:        "category",
						Aliases:     []string{"c"},
						Required:    true,
						Destination: &reclassifyCategory,
						Usage:       "category to move the note into",
					},
				},
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					if cmd.NArg() < 1 {
						return ctx, styledExit(fmt.Errorf("usage: park reclassify <file> --category <category>"), 2)
					}
					if !cfg.HasCategory(reclassifyCategory) {
						return ctx, styledExit(cfg.UnknownCategoryError(reclassifyCategory), 2)
					}
					return ctx, nil
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := store.Reclassify(cfg, cmd.Args().First(), reclassifyCategory); err != nil {
						return styledExit(err, 1)
					}
					return nil
				},
			},
			{
				Name:  "schema",
				Usage: "print the frontmatter schema contract",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:        "json",
						Destination: &schemaJSON,
						Usage:       "output machine-readable JSON",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if err := schemaPark(schemaJSON, cmd.Root().Writer); err != nil {
						return styledExit(err, 1)
					}
					return nil
				},
			},
			{
				Name:      "show",
				Usage:     "render a note to the terminal",
				ArgsUsage: "<file>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:        "plain",
						Destination: &showPlain,
						Usage:       "force plain text output (default when output is not a terminal)",
					},
				},
				Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
					if cmd.NArg() < 1 {
						return ctx, styledExit(fmt.Errorf("usage: park show <file>"), 2)
					}
					return ctx, nil
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					path, err := store.ResolvePath(cfg, cmd.Args().First())
					if err != nil {
						return styledExit(err, 1)
					}
					plain := showPlain || !writerIsTTY(cmd.Root().Writer)
					if err := render.ShowFile(path, cmd.Root().Writer, plain); err != nil {
						return styledExit(err, 1)
					}
					return nil
				},
			},
		},
	}

}

// styledExit wraps an error in the configured styled output and returns a
// CLI exit error with the supplied code.
func styledExit(err error, code int) error {
	return cli.Exit(styledError(err), code)
}

// StyledError returns a user-facing error string, styled when stdout is a
// terminal.
func styledError(e error) string {
	if e == nil {
		return ""
	}
	if !isTerminal(os.Stderr) {
		return e.Error()
	}
	header := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.CharmYellow)).
		Bold(true).
		Render("HEAVENS TO MURGATROYD!")
	body := lipgloss.NewStyle().
		Foreground(lipgloss.Color(theme.CharmRed)).
		Render(theme.CurrentGlyphs().ErrorBullet, e.Error())
	return header + "\n" + body
}
