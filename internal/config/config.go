// Package config reads the settings file that persists what a flag cannot:
// the repositories the Repos tab lists, the saved search queries, and the
// choices a user makes once rather than every run.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// Config is the settings file. Every field is optional; the zero Config is
// what someone with no settings file gets, and every feature works from it.
type Config struct {
	Language   string `yaml:"language,omitempty"`
	Icons      string `yaml:"icons,omitempty"`
	DefaultTab string `yaml:"default_tab,omitempty"`

	// Repositories is the list the Repos tab shows, in the order it shows
	// them.
	Repositories []string `yaml:"repositories,omitempty"`
}

// WantsRepos reports whether default_tab asks to start on the Repos tab.
// It is as tolerant of case and surrounding whitespace as icon.Resolve and
// i18n.Resolve are of their own values.
func (c Config) WantsRepos() bool {
	return strings.EqualFold(strings.TrimSpace(c.DefaultTab), "repos")
}

// Path is where the settings file lives: octoscope/config.yaml under the
// directory the operating system keeps configuration in (%AppData% on
// Windows, ~/Library/Application Support on macOS, ~/.config on Linux).
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the config directory: %w", err)
	}
	return filepath.Join(dir, "octoscope", "config.yaml"), nil
}

// Load reads the settings file. A file that is not there is not a failure:
// the answer is the zero Config. A file that is there but unreadable returns
// the zero Config alongside the error, so a caller can report it and carry
// on with defaults rather than refusing to start.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return c, nil
}

// Store reads and writes one settings file.
type Store struct{ path string }

// NewStore returns a store for the settings file at path.
func NewStore(path string) *Store { return &Store{path: path} }

// SaveRepositories replaces the repository list and leaves every other
// setting as it was. A file that cannot be parsed is not written at all: a
// list is not worth flattening the rest of someone's settings for.
func (s *Store) SaveRepositories(repos []string) error {
	// main builds a store even when it could not locate the config
	// directory, so that the failure surfaces here rather than as a save
	// that silently does nothing.
	if s.path == "" {
		return errors.New("no settings file to write: the config directory could not be located")
	}
	c, err := Load(s.path)
	if err != nil {
		return err
	}
	c.Repositories = repos
	return s.save(c)
}

// save writes through a temporary file in the same directory so that an
// interrupted write cannot leave a half-written settings file behind.
func (s *Store) save(c Config) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode %s: %w", s.path, err)
	}
	tmp, err := os.CreateTemp(dir, "config-*.yaml")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	// Removing the temporary file fails once the rename below has moved it,
	// which is the successful path and not something to report.
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close() // the write already failed; the file is about to go
		return fmt.Errorf("write %s: %w", tmp.Name(), err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmp.Name(), err)
	}
	if err := os.Rename(tmp.Name(), s.path); err != nil {
		return fmt.Errorf("replace %s: %w", s.path, err)
	}
	return nil
}
