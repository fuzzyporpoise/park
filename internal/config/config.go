// Package config defines the park configuration schema, validation, and
// default values.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"go.fuzzyporpoise.dev/park/internal/fs"
	"go.fuzzyporpoise.dev/park/schema"
)

// Config is the top-level configuration for park.
type Config struct {
	DefaultCategory string     `toml:"default_category"`
	Categories      []Category `toml:"category"`
}

// LoadConfig reads the config file from the canonical path. If the file
// doesn't exist, it fills the receiver with the default config.
func (c *Config) LoadConfig(root, configPath string) error {
	if c == nil {
		return fmt.Errorf("config receiver is nil")
	}

	configPath, err := fs.ExpandPath(configPath)
	if err != nil {
		return fmt.Errorf("expand config path: %w", err)
	}
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		*c = *DefaultConfig(root)
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config %q: %w", configPath, err)
	}

	if err := toml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parse config %q: %w", configPath, err)
	}

	// Expand ~ in paths
	for i := range c.Categories {
		path, err := fs.ExpandPath(c.Categories[i].Path)
		if err != nil {
			return fmt.Errorf("expand category path %q: %w", c.Categories[i].Path, err)
		}
		c.Categories[i].Path = path
	}

	if err := c.Validate(); err != nil {
		return fmt.Errorf("validate config %q: %w", configPath, err)
	}

	return nil
}

// Validate checks that the config is well-formed.
func (c *Config) Validate() error {
	if c.DefaultCategory == "" {
		return fmt.Errorf("default_category is required")
	}
	if !c.HasCategory(c.DefaultCategory) {
		return fmt.Errorf("default_category %q does not match any category name", c.DefaultCategory)
	}

	seenNames := make(map[string]struct{})
	seenKeys := make(map[string]struct{})
	for _, cl := range c.Categories {
		if cl.Name == "" {
			return fmt.Errorf("category name cannot be empty")
		}
		if _, ok := seenNames[cl.Name]; ok {
			return fmt.Errorf("duplicate category name %q", cl.Name)
		}
		seenNames[cl.Name] = struct{}{}

		if cl.Key != "" {
			if _, ok := seenKeys[cl.Key]; ok {
				return fmt.Errorf("duplicate key %q for category %q", cl.Key, cl.Name)
			}
			seenKeys[cl.Key] = struct{}{}
		}
	}
	return nil
}

// HasCategory reports whether a category with the given name exists.
func (c *Config) HasCategory(name string) bool {
	for _, cl := range c.Categories {
		if cl.Name == name {
			return true
		}
	}
	return false
}

// CategoryByName returns the category with the given name, or zero value if not found.
func (c *Config) CategoryByName(name string) (Category, bool) {
	for _, cl := range c.Categories {
		if cl.Name == name {
			return cl, true
		}
	}
	return Category{}, false
}

// CategoryNames returns all category names in order.
func (c *Config) CategoryNames() []string {
	names := make([]string, len(c.Categories))
	for i, cl := range c.Categories {
		names[i] = cl.Name
	}
	return names
}

// UnknownCategoryError is the single constructor for "unknown category"
// errors, so every command and package reports the same message.
func (c *Config) UnknownCategoryError(name string) error {
	return fmt.Errorf("unknown category %q; valid: %s", name, strings.Join(c.CategoryNames(), ", "))
}

// Dump returns the config as a TOML string.
func (c *Config) Dump() (string, error) {
	var b strings.Builder
	enc := toml.NewEncoder(&b)
	if err := enc.Encode(c); err != nil {
		return "", fmt.Errorf("encode config: %w", err)
	}
	return b.String(), nil
}

// DefaultConfigPathFor returns the default configuration path under the given
// root. An empty root falls back to DefaultRootPath().
func DefaultConfigPathFor(root string) string {
	if root == "" {
		root = DefaultRootPath()
	}
	return filepath.Join(root, "config")
}

// DefaultRootPath returns the park root directory.
//
// Resolution order:
//  1. $XDG_CONFIG_HOME/park if $XDG_CONFIG_HOME is set.
//  2. os.UserConfigDir()/park, following the OS convention
//     (e.g. ~/Library/Application Support/park on macOS,
//     ~/.config/park on Linux and other Unix systems).
//  3. $HOME/.config/park if os.UserConfigDir fails.
func DefaultRootPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "park")
	}

	rootDir, err := os.UserConfigDir()
	if err != nil {
		home, homeErr := fs.ExpandPath("~")
		if homeErr != nil {
			return ""
		}
		rootDir = filepath.Join(home, ".config")
	}
	return filepath.Join(rootDir, "park")
}

// Category defines a single category (inbox, project, area, archive, or
// user-defined) with its storage path and TUI hotkey. Excluded categories are
// omitted from `park list` by default and reappear with `--all`.
type Category struct {
	Name     string `toml:"name"`
	Path     string `toml:"path"`
	Key      string `toml:"key"`
	Excluded bool   `toml:"excluded,omitempty"`
}

// DefaultConfig returns the built-in IPAA default configuration.
func DefaultConfig(root string) *Config {
	if root == "" {
		root = DefaultRootPath()
	}
	return &Config{
		DefaultCategory: string(schema.CategoryInbox),
		Categories: []Category{
			{Name: string(schema.CategoryInbox), Path: filepath.Join(root, "_inbox"), Key: "i"},
			{Name: string(schema.CategoryProjects), Path: filepath.Join(root, "_projects"), Key: "p"},
			{Name: string(schema.CategoryAreas), Path: filepath.Join(root, "_areas"), Key: "a"},
			{Name: string(schema.CategoryArchive), Path: filepath.Join(root, "_archive"), Key: "x", Excluded: true},
		},
	}
}
