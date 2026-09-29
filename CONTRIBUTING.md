# Contributing

## Prerequisites

- Go 1.26.4+
- `golangci-lint` for `make lint` (install with `make deps`)

## Getting started

```bash
git clone https://github.com/polymorcodeus/park.git
cd park
make build
```

## Before opening a PR

```bash
make check    # fmt, vet, lint, test
```

- Keep PRs focused: one behavior change per PR.
- Update `README.md` and `CONTRIBUTING.md` when behavior, commands, or flags change; docs are part of done.
- Follow the conventions in this file (package boundaries, error wrapping, message ownership).

## Package boundaries

| Package | Does | Imports |
|---------|------|---------|
| `schema` | public frontmatter contract; canonical categories, version, and write template | stdlib only |
| `internal/config` | configuration schema, loading, validation | `internal/fs`, `schema` |
| `internal/note` | note content, frontmatter parsing/writing, creation, ingestion (`Note`, `Parse`, `Write`, `Add`, `Create`) | `internal/config`, `internal/fs`, `schema` |
| `internal/store` | on-disk item management, scanning, reclassification, list formatting | `internal/config`, `internal/note`, `schema` |
| `internal/render` | glamour-based rendering | `internal/note` |
| `internal/theme` | color constants | stdlib only |
| `internal/fs` | filesystem helpers (ExpandPath) | stdlib only |
| `internal/model` | Bubble Tea TUI screens | `internal/config`, `internal/note`, `internal/store`, `internal/theme` |
| `cmd/park` | CLI tree + wiring | everything |

## Conventions

These rules keep the package boundaries above meaningful as the codebase grows.

### File I/O and frontmatter

- `internal/fs` is the only package that expands `~` and `$HOME`. Ingestion
  paths (`Draft.FromFile`) are normalized exactly once, in `note.IngestFile`,
  so parsing, form preview, and source-file removal all see the same path.
- `internal/note` owns all frontmatter parsing and writing. Code outside this
  package should not parse `---` blocks by hand.
- `internal/store` owns category-folder operations, resolving filenames to
  full paths, and `park list` grouping/formatting (plain and JSON). Filename
  resolution is unified in `ResolvePath`: a bare basename is searched across
  every configured category folder, while a value containing a path separator
  is treated as a literal path and never joined onto a category folder.
  `park show` and `park reclassify` both resolve their `<file>` argument this
  way, so a path can never double-join.
- `internal/model` may call `note.Create`, `store.Scan`, and `store.Reclassify`,
  but should not read files directly from disk except through those packages.
- `cmd/park` parses CLI flags and delegates all file/content work to
  `internal/note` or `internal/store`.

### Data models

- `schema.Frontmatter` is the canonical metadata block: `Category`,
  `Created`, `Source`, `Synopsis`. It lives in the public `schema` package so
  downstream tooling can import the contract instead of re-deriving it.
- `note.Metadata` is an alias to `schema.Frontmatter`. It is not validated in
  isolation because its completeness depends on context.
- `note.Draft` is the creation-time model: `Filename`, `Body`, `FromFile`,
  plus `Metadata`. `Created` may be empty; it is populated when the draft is
  converted to a note. `Draft.ReadyToCreate()` checks `Filename`,
  `Metadata.Category`, `Metadata.Source`, and `Metadata.Synopsis`.
- `note.Note` is the persisted model: `Path`, `Body`, plus complete
  `Metadata` (`Created` always set). Completeness checks go through
  `schema.Frontmatter.IsComplete()`; there is no `Note`-level completeness
  wrapper.
- `store.Item` is the read/scanned model. It embeds `note.Metadata` plus
  `Path`, `Filename`, and `ModTime`.
- `store.Group` is a category name plus its `[]Item`; `store.List` returns
  groups in config order and `store.FormatList`/`store.WriteListJSON` render
  them for `park list`.
- `Body` has the same meaning in `Draft` and `Note` (markdown content below
  the frontmatter). During `Draft` → `Note` conversion, any embedded frontmatter
  in `Body` is stripped and merged into `Metadata`, so `Note.Body` is always
  clean.

### TUI

- Key bindings are matched in `Update` from the same `key.Binding` values that
  render help (see `categoryBinding` in `internal/model/assist.go`, which pairs
  each category name with its binding), so help text and behavior cannot drift.

### Errors and output

- Errors are wrapped with `fmt.Errorf("...: %w", err)` when crossing package
  boundaries.
- User-facing message formatting belongs in the package that owns the data.
  `cmd/park` wires output to the terminal but does not format store results.
- Category-validation errors are constructed once, by
  `Config.UnknownCategoryError`; CLI `Before` hooks and domain packages call it
  instead of hand-writing the "unknown category" message.
- Avoid `init()` for logic that can be explicit in `main()` or a constructor.
- Exported helpers with no callers get deleted, not kept "just in case": the
  `unused` linter cannot see exported identifiers, so dead API only surfaces
  in review.

## Styling

The TUI uses the Charm design system tokens:

- Page background: `#14121a`
- Raised surface: `#1c1a24`
- Primary text: `#f5f1fa`
- Muted text: `#a79fc0`
- Faint text: `#6f6785`
- Accent purple: `#7d56f4`
- Accent pink: `#FF4081`

Colors are defined as exported constants in `internal/theme/theme.go` so both
the TUI (`internal/model`) and the CLI's styled error output (`cmd/park`)
share the same palette.

## Implementation notes

- Frontmatter is flat `key: value` parsed line-by-line (no YAML dependency).
- `Reclassify` rewrites frontmatter *before* moving the file, so a failed move never
  leaves a file in an inconsistent state.
- `schema` is a public, stdlib-only package. Other tools can import
  `go.fuzzyporpoise.dev/park/schema` to pin the frontmatter contract.
- `internal/config` has no dependency on `main.go`; importing the focused
  packages into another CLI is just wiring commands to the exported functions.
- `Config` is loaded once in the CLI `Before` hook and passed as `*config.Config`
  to all command helpers.
