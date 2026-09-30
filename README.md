<p align="center">
  <source media="(prefers-color-scheme: dark)" srcset="images/park-dark.png">
  <source media="(prefers-color-scheme: light)" srcset="images/park-light.png">
  <img alt="Project Logo" src="images/park-dark.png" width="128">
</p>

# park

[![Go Version](https://img.shields.io/github/go-mod/go-version/fuzzyporpoise/park)](https://go.dev/) [![Build Status](https://img.shields.io/github/actions/workflow/status/fuzzyporpoise/park/ci.yml?branch=main)](https://github.com/fuzzyporpoise/park/actions) [![License](https://img.shields.io/github/license/fuzzyporpoise/park)](./LICENSE) [![Go Reference](https://pkg.go.dev/badge/go.fuzzyporpoise.dev/park/schema.svg)](https://pkg.go.dev/go.fuzzyporpoise.dev/park/schema)

**A parking lot for markdown notes, organized as IPAA (Inbox / Projects / Areas / Archive).**

Part of the [fuzzyporpoise](https://github.com/fuzzyporpoise) suite of CLI tooling for dotfile and knowledge management.

Park surfaces notes mid-coding-session and keeps the Inbox skim-able through frontmatter synopses. Movement between the four categories is a bidirectional *category decision*, not a linear workflow: a Project that turns out to be open-ended becomes an Area, an Area that gets scoped down becomes a Project, and either can go stale into Archive. The repo itself is structured as a standalone Go module with importable `internal/*` packages, designed to be wired into a larger personal CLI.

## Quick Demo

```bash
park init
park new "revisit dashboard caching approach" \
  -s "current TTL feels wrong, worth a spike" -src my-cli-tool
park new "keep an eye on API rate limits" -c areas
park list
park assist
park reclassify 1767786622-idea.md -c projects
```

## Install

Quick install (downloads the latest release to `/usr/local/bin`):

```bash
curl -sSL https://raw.githubusercontent.com/fuzzyporpoise/park/main/install.sh | bash
```

The `go install` and source builds require Go 1.26.4+.

Or install via Go:

```bash
go install go.fuzzyporpoise.dev/park@latest
```

Or build from source:

```bash
git clone https://github.com/fuzzyporpoise/park.git
cd park
make build
```

## Setup

```bash
park init     # creates _inbox/, _projects/, _areas/, _archive/
```

`park init` is idempotent: running it again reports which folders already exist and only creates missing ones. To verify initialization without changing anything -- useful in setup scripts and CI -- use `park check`:

```bash
park check    # exits 0 when all category folders exist, 1 otherwise
```

The default root is the OS user config directory (`~/.config/park` on Linux, `~/Library/Application Support/park` on macOS). Scope notes to a specific location by setting `PARK_ROOT`:

```bash
export PARK_ROOT="$(pwd)/.park"
```

## Storage

Notes are plain markdown files with frontmatter, one folder per category:

```bash
~/.config/park/
├── config              # TOML config
├── _inbox/             # default landing zone
├── _projects/          # bounded work with a defined end state
├── _areas/             # ongoing responsibilities
└── _archive/           # inactive, retained for reference
```

Layout shown for the Linux default root; on macOS the root is `~/Library/Application Support/park`.

### Frontmatter

```text
---
category: inbox
created: 2026-07-16
source: terminal
synopsis: current TTL feels wrong, worth a spike
---
```

Frontmatter is parsed line-by-line -- no YAML dependency. Four fields:

| field | purpose |
|-------|---------|
| `category` | redundant with folder, kept so ad-hoc files still self-describe |
| `created` | ISO 8601 date, set at park time |
| `source` | where the note came from (repo, chat, stray idea) |
| `synopsis` | one-line summary -- the entire mechanism that makes triage fast |

`synopsis` is the key design decision: read it, decide whether to open the file, move on. `source` lets future-you reconstruct *why* a note exists without re-reading it.

### Schema contract

The frontmatter contract is published as a public, stdlib-only Go package at `go.fuzzyporpoise.dev/park/schema` and exposed through the CLI:

```bash
park schema        # human-readable contract
park schema --json # machine-readable contract for downstream tooling
```

The JSON output includes the canonical category enum, field kinds, the date format, and the exact write template. Import the package directly when building tools that write or validate park notes.

### Reclassification

`park reclassify <file> -c <category>` rewrites frontmatter *before* moving the file. A failed move never leaves a note in a half-updated state. Same-category moves are rejected.

`<file>` follows the same resolution rules as `park show`: a bare basename is searched across every configured category folder (archive included), or a path (absolute, or relative to the working directory) is used as-is. A value containing a path separator is treated as a literal path and is never joined onto a category folder, so passing a path cannot double-join into `category/path/to/file.md`.

## Commands

| command | purpose |
|---------|---------|
| `init` | create the category folders (idempotent) |
| `check` | verify category folders exist (useful for automation) |
| `new [title]` | park a new note (alias `add`) |
| `list [--all] [-c <cat>] [--json]` | list parked notes grouped by category (alias `ls`) |
| `assist` | open the tabbed TUI browser |
| `show <file> [--plain]` | render a note to the terminal (plain when piped, or with `--plain`) |
| `reclassify <file> -c <cat>` | reclassify a note (alias `recat`) |
| `config` | print the loaded TOML config |
| `schema` | print the frontmatter schema contract |
| `schema --json` | print the contract as machine-readable JSON |

### `park new` options

| option | purpose |
|--------|---------|
| `-s, --synopsis` | one-line summary (what it is / why it matters) |
| `-src, --source` | origin of the note (repo name, chat, etc.) |
| `-c, --category` | park directly into a specific category |
| `-f, --from-file` | ingest an existing markdown file |
| `--filename` | explicit file slug |

### `park list` options

| option | purpose |
|--------|---------|
| `--all` | include categories excluded by config (for example archive) |
| `-c, --category` | only list these categories (repeatable; overrides exclusion) |
| `--json` | emit a versioned, machine-readable envelope |

`park list` renders one block per category: an upper-cased banner followed by each note's filename and frontmatter fields. Notes within a category are ordered oldest-modified first, and categories marked `excluded` in the config are hidden until `--all` is passed or the category is named explicitly with `-c`.

```
INBOX
-----

filename: ttl-spike.md
category: inbox
created: 2026-07-16
source: terminal
synopsis: current TTL feels wrong, worth a spike

PROJECTS
--------

filename: caching.md
category: projects
created: 2026-07-18
source: my-cli-tool
synopsis: dashboard caching rework
```

`park list --json` flattens every group into a single `items` array, each item carrying its `category`, `path`, and RFC 3339 `modified` timestamp, wrapped in a versioned envelope:

```json
{
  "schema_version": 1,
  "items": [
    {
      "filename": "ttl-spike.md",
      "path": "/home/you/.config/park/_inbox/ttl-spike.md",
      "category": "inbox",
      "created": "2026-07-16",
      "source": "terminal",
      "synopsis": "current TTL feels wrong, worth a spike",
      "modified": "2026-07-16T19:03:11Z"
    }
  ]
}
```

`schema_version` tracks the frontmatter contract exposed by `park schema`, so consumers can detect drift.

### `park show` options

| option | purpose |
|--------|---------|
| `--plain` | force plain text output (no ANSI); auto-selected when stdout is not a terminal |

`park show` renders a note's frontmatter summary and body through glamour. When stdout is not a terminal (piped to a file, a pager, or another process) it automatically emits plain text with no ANSI escape codes, so captured output stays readable; `--plain` forces that mode even on a terminal. `park assist` applies the same rule to the note it opens after the TUI exits.

### Ingestion

`park new` accepts content through two non-interactive paths:

**From a file** (`-f, --from-file`):

```bash
park new -f ~/Downloads/meeting-notes.md \
  -s "Q3 planning recap" -src "Slack export"
```

The file is read, wrapped with frontmatter, and moved into the park; the original file is removed.

**From stdin** (pipe):

```bash
cat research-summary.md | park new "llm context window research" \
  -s "survey of recent papers" -src "Claude"
```

When stdin is not a terminal, `park new` reads the entire input as the note body. If a filename or synopsis is also provided on the command line, the note is parked immediately -- no TUI form opens. When content is piped in without enough metadata to build a complete note, the TUI form opens pre-filled with the piped body for interactive completion.

### Global options

| option | env | default | purpose |
|--------|-----|---------|---------|
| `--park-root <path>` | `PARK_ROOT` | OS user config dir/park | root directory for parked notes |
| `--park-config <path>` | `PARK_CONFIG` | `<root>/config` | path to the config file |

## TUI

`park assist` opens a tabbed TUI browser across all configured categories, with interactive moves between categories.

| key | action |
|-----|--------|
| `↑`/`↓` | move selection |
| `enter`/`v` | view full note (glamour render) |
| `→`/`l` / `ctrl+n` | next category |
| `←`/`h` / `ctrl+p` | previous category |
| `?` | toggle full help |
| `i`/`p`/`a`/`x` | reclassify selected note to inbox/projects/areas/archive |
| `/` | filter (category shortcuts disabled while typing) |
| `esc` | cancel filter when filtering |
| `q` / `ctrl+c` | quit |

## Agentic workflows

`park new` supports two ingestion paths that fit naturally into agentic pipelines -- AI-generated summaries, chat logs, coding session captures, or any markdown emitted by another tool.

### Pipe from an agent or script

Any process that writes markdown to stdout can park it in one command:

```bash
# Park a coding session summary generated by an LLM
llm "summarize what we just did" | park new "session recap $(date +%F)" \
  -s "$(llm 'one-line summary of this session')" -src "Claude"

# Park a markdown report from a script
./scripts/generate-daily-report.sh | park new "daily report $(date +%F)" \
  -s "automated daily metrics" -src "cron"
```

When piped content arrives with a filename and synopsis, the note is parked immediately with no TUI -- ideal for unattended scripts, cron jobs, or agent tool calls.

### Ingest an existing file

Markdown files produced by other tools land in the park without manual copying:

```bash
park new -f /tmp/claude-output.md \
  -s "database schema redesign proposal" -src "Claude"
```

The file is moved into the category folder and the original is removed, same as any `--from-file` ingest; see [Ingestion](#ingestion).

### Verify setup from an agent

Before running other commands, an agent can ensure the park storage is initialized without side effects:

```bash
park check || park init
```

`park check` exits 0 when all configured category folders exist and exits 1 (printing the missing paths) when they do not. This makes it safe to call idempotently in setup scripts and agent tool loops.

### Reclassify from an agent

Automation does not have to stop at ingestion. A scheduled job or agent can reclassify stale inbox items:

```bash
# Move everything older than 30 days from inbox to archive
find "$PARK_ROOT/_inbox" -name "*.md" -mtime +30 -exec park reclassify {} -c archive \;
```

### Read a note from an agent

`park show` writes to stdout and drops styling when output is redirected, so captured text is clean:

```bash
park show inbox-note.md | grep -i "deadline"
```

## Configuration

Config lives at `<park-root>/config` as TOML. Run `park config` to print the loaded config (the built-in default when no config file exists):

```toml
default_category = "inbox"

[[category]]
  name = "inbox"
  path = "~/.config/park/_inbox"
  key = "i"

[[category]]
  name = "projects"
  path = "~/.config/park/_projects"
  key = "p"

[[category]]
  name = "areas"
  path = "~/.config/park/_areas"
  key = "a"

[[category]]
  name = "archive"
  path = "~/.config/park/_archive"
  key = "x"
  excluded = true
```

Categories are fully configurable: name, storage path, and TUI hotkey. Add or remove categories to match your workflow. `default_category` is where `park new` lands notes when `-c` is omitted.

Set `excluded = true` on a category to hide it from `park list` by default; it reappears with `park list --all` or when named explicitly (`park list -c archive`). The built-in config ships with `archive` excluded.

## Tech stack

| component | library |
|-----------|---------|
| TUI framework | [Bubble Tea](https://github.com/charmbracelet/bubbletea) |
| TUI components | [Bubbles](https://github.com/charmbracelet/bubbles) |
| styling | [Lip Gloss](https://github.com/charmbracelet/lipgloss) |
| markdown rendering | [Glamour](https://github.com/charmbracelet/glamour) |
| CLI framework | [urfave/cli/v3](https://github.com/urfave/cli) |
| config format | [BurntSushi/toml](https://github.com/BurntSushi/toml) |

## Contributing

```bash
git clone https://github.com/fuzzyporpoise/park.git
cd park
make check    # fmt, vet, lint, test
```

See [CONTRIBUTING.md](./CONTRIBUTING.md) for the full setup.

## License

[MIT](./LICENSE)
