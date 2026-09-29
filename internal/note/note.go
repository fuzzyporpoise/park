// Package note handles parked-note content: frontmatter, creation, ingestion,
// and the headless/interactive decision path.
package note

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"go.fuzzyporpoise.dev/park/internal/config"
	"go.fuzzyporpoise.dev/park/internal/fs"
	"go.fuzzyporpoise.dev/park/schema"
)

// Metadata is the shared metadata block for parked notes. It is an alias
// so the canonical contract lives in one place: the public schema package.
type Metadata = schema.Frontmatter

// Note is the persisted representation of a parked note. Path is empty when
// the note is parsed from a string rather than read from a file.
type Note struct {
	Body string
	Path string
	Metadata
}

// Draft is the creation-time representation of a note. Created may be empty
// and is populated when the draft is converted to a Note.
type Draft struct {
	Filename string
	Body     string
	FromFile string
	Metadata
}

// WithDefaults fills in the default category and derives the filename from
// the source file when either is empty.
func (d Draft) WithDefaults(cfg *config.Config) Draft {
	if d.Category == "" {
		d.Category = cfg.DefaultCategory
	}
	if d.Filename == "" && d.FromFile != "" {
		d.Filename = filepath.Base(d.FromFile)
	}
	return d
}

// ReadyToCreate reports whether the draft has all fields required to create a
// note. Created is intentionally not checked; it is populated on write.
func (d Draft) ReadyToCreate() bool {
	return fieldSet(d.Filename) && fieldSet(d.Category) && fieldSet(d.Source) && fieldSet(d.Synopsis) && d.Slug() != ""
}

// MissingFields returns the user-supplied fields still required to create a note.
func (d Draft) MissingFields() []string {
	var missing []string
	if !fieldSet(d.Filename) || !fieldSet(d.Slug()) {
		missing = append(missing, "filename")
	}
	if !fieldSet(d.Category) {
		missing = append(missing, "category")
	}
	if !fieldSet(d.Source) {
		missing = append(missing, "source")
	}
	if !fieldSet(d.Synopsis) {
		missing = append(missing, "synopsis")
	}
	return missing
}

// Slug returns a URL-safe slug derived from the draft filename.
func (d Draft) Slug() string {
	return slugify(d.Filename)
}

// H1 scans the body for a single-line H1 heading and returns the heading text.
func (d Draft) H1() (string, bool) {
	for line := range strings.SplitSeq(d.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if after, ok := strings.CutPrefix(trimmed, "# "); ok {
			return strings.TrimSpace(after), true
		}
		break
	}
	return "", false
}

// Parse reads the leading `---` delimited block from a markdown file and
// returns the parsed note. The note's Path is set to the file path.
// Unknown frontmatter keys are ignored.
func Parse(path string) (Note, error) {
	f, err := os.Open(path)
	if err != nil {
		return Note{}, fmt.Errorf("open %q: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	n, _, err := parse(newScanner(f))
	if err != nil {
		return n, fmt.Errorf("parse %q: %w", path, err)
	}
	n.Path = path
	return n, nil
}

// ParseString parses the leading `---` delimited block from a markdown string
// and returns the parsed note, a flag indicating whether a frontmatter block
// was found, and any parse error. Unknown frontmatter keys are ignored.
func ParseString(content string) (Note, bool, error) {
	n, found, err := parse(newStringScanner(content))
	if err != nil {
		return n, found, fmt.Errorf("parse string: %w", err)
	}
	return n, found, nil
}

func newScanner(f *os.File) *bufio.Scanner { return bufio.NewScanner(f) }

func newStringScanner(s string) *bufio.Scanner { return bufio.NewScanner(strings.NewReader(s)) }

func parse(scanner *bufio.Scanner) (Note, bool, error) {
	n := Note{}

	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return n, false, err
		}
		return n, false, nil
	}
	firstLine := scanner.Text()
	if strings.TrimSpace(firstLine) != "---" {
		bodyLines := []string{firstLine}
		for scanner.Scan() {
			bodyLines = append(bodyLines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return n, false, err
		}
		n.Body = strings.TrimLeft(strings.Join(bodyLines, "\n"), "\n")
		return n, false, nil
	}

	var bodyLines []string
	inBlock := true
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			inBlock = false
			continue
		}
		if inBlock {
			key, val, ok := splitKV(line)
			if !ok {
				continue
			}
			switch key {
			case "category":
				n.Category = val
			case "created":
				n.Created = val
			case "source":
				n.Source = val
			case "synopsis":
				n.Synopsis = val
			}
			continue
		}
		bodyLines = append(bodyLines, line)
	}
	if err := scanner.Err(); err != nil {
		return n, false, err
	}
	n.Body = strings.TrimLeft(strings.Join(bodyLines, "\n"), "\n")
	return n, true, nil
}

func splitKV(line string) (key, val string, ok bool) {
	key, val, ok = strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	val = strings.TrimSpace(val)
	return key, val, key != ""
}

// Write writes (or overwrites) a markdown file with the note's frontmatter
// and body. It writes to a temp file and renames into place so a crash
// mid-write never leaves a partially-written note.
func Write(path string, n Note) (err error) {
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp %q: %w", tmpPath, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = fmt.Fprint(f, schema.Render(n.Metadata, n.Body)); err != nil {
		return fmt.Errorf("write temp %q: %w", tmpPath, err)
	}

	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename %q to %q: %w", tmpPath, path, err)
	}
	return nil
}

// Today returns the current date in the frontmatter's date format.
func Today() string {
	return time.Now().Format(schema.DateFormat)
}

// Result is the outcome of attempting to add a note headlessly.
// Exactly one of Path or Form is set.
type Result struct {
	Path string
	Form *Draft
}

// IngestFile reads the source file into the draft body when FromFile is set
// and Body is empty, merging any file frontmatter metadata with the draft's
// existing metadata (draft values take precedence). FromFile is expanded once
// here, so parsing, form preview, and source removal all see the same path.
func IngestFile(d Draft) (Draft, error) {
	if d.FromFile == "" || d.Body != "" {
		return d, nil
	}
	expanded, err := fs.ExpandPath(d.FromFile)
	if err != nil {
		return Draft{}, fmt.Errorf("expand source path %q: %w", d.FromFile, err)
	}
	d.FromFile = expanded
	parsed, err := Parse(d.FromFile)
	if err != nil {
		return Draft{}, fmt.Errorf("parse source file %q: %w", d.FromFile, err)
	}
	if d.Category == "" {
		d.Category = parsed.Category
	}
	if d.Source == "" {
		d.Source = parsed.Source
	}
	if d.Synopsis == "" {
		d.Synopsis = parsed.Synopsis
	}
	d.Body = parsed.Body
	return d, nil
}

// Add decides whether a note can be created headlessly or needs the
// interactive form. It parses the source file and body frontmatter when
// present, merging metadata with CLI values taking precedence, then applies
// config defaults for anything still empty.
func Add(cfg *config.Config, d Draft) (Result, error) {
	if d.FromFile != "" && d.Body != "" {
		return Result{}, fmt.Errorf("cannot specify both --from-file and a body")
	}

	if d.FromFile != "" {
		var err error
		d, err = IngestFile(d)
		if err != nil {
			return Result{}, err
		}
	} else if d.Body != "" {
		parsed, hasFM, err := ParseString(d.Body)
		if err != nil {
			return Result{}, err
		}
		if hasFM {
			if d.Category == "" {
				d.Category = parsed.Category
			}
			if d.Source == "" {
				d.Source = parsed.Source
			}
			if d.Synopsis == "" {
				d.Synopsis = parsed.Synopsis
			}
			missing := slices.DeleteFunc(d.Metadata.MissingFields(), func(f string) bool { return f == "created" })
			if len(missing) > 0 {
				return Result{}, fmt.Errorf("incomplete frontmatter: missing %s", strings.Join(missing, ", "))
			}
			d.Body = parsed.Body
		}
	}

	d = d.WithDefaults(cfg)

	if d.ReadyToCreate() {
		path, err := Create(cfg, d)
		if err != nil {
			return Result{}, err
		}
		return Result{Path: path}, nil
	}

	if d.Body != "" || d.FromFile != "" {
		if d.Filename == "" {
			title, ok := d.H1()
			if !ok {
				return Result{}, fmt.Errorf("missing filename, retry with --filename")
			}
			d.Filename = title
			if d.ReadyToCreate() {
				path, err := Create(cfg, d)
				if err != nil {
					return Result{}, err
				}
				return Result{Path: path}, nil
			}
		}
	}

	return Result{Form: &d}, nil
}

// Create writes a note from a complete draft. Callers (Add and the form) are
// responsible for ensuring required fields are present; this function resolves
// the category path, writes the note, and removes the source file if FromFile
// is set.
func Create(cfg *config.Config, d Draft) (string, error) {
	d = d.WithDefaults(cfg)

	cl, ok := cfg.CategoryByName(d.Category)
	if !ok {
		return "", cfg.UnknownCategoryError(d.Category)
	}

	path := filepath.Join(cl.Path, d.Slug()+".md")

	if _, err := os.Stat(cl.Path); os.IsNotExist(err) {
		return "", fmt.Errorf("category folder %q does not exist; run `park init` to create it", cl.Path)
	}

	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("note already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("check note path %q: %w", path, err)
	}

	n := Note{
		Path: path,
		Body: d.Body,
		Metadata: schema.Frontmatter{
			Category: d.Category,
			Created:  Today(),
			Source:   d.Source,
			Synopsis: d.Synopsis,
		},
	}

	if err := Write(path, n); err != nil {
		return "", fmt.Errorf("write note %q: %w", path, err)
	}

	if d.FromFile != "" {
		srcPath, err := fs.ExpandPath(d.FromFile)
		if err != nil {
			return "", fmt.Errorf("expand source path %q: %w", d.FromFile, err)
		}
		if err := os.Remove(srcPath); err != nil {
			return "", fmt.Errorf("remove source file %q: %w", srcPath, err)
		}
	}
	return path, nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s, _ = strings.CutSuffix(s, ".md")
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func fieldSet(s string) bool {
	return strings.TrimSpace(s) != ""
}
