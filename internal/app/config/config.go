// Package config is the shape of the settings file and how to read it. What
// the application does with the data inside is not this package's business:
// internal/app/adapter/datasource owns that, and writes the file back.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
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

	// SavedQueries is the Search tab's saved queries, in the order the user
	// saved them.
	SavedQueries []SavedQueryEntry `yaml:"saved_queries,omitempty"`
}

// SavedQueryEntry is one entry of saved_queries as the file spells it. The
// application's own type is domain.SavedQuery; this one exists to carry the
// yaml tags, which the domain does not have.
type SavedQueryEntry struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}

// DefaultTabName is the tab default_tab asks to start on, normalised. A name
// no tab answers to reads as unset: a typo in a setting is not a reason to
// refuse to start.
func (c Config) DefaultTabName() string {
	switch name := strings.ToLower(strings.TrimSpace(c.DefaultTab)); name {
	case "repos", "search":
		return name
	default:
		return ""
	}
}

// Path is where the settings file lives: octoscope/config.yaml under
// %AppData% on Windows, and under $XDG_CONFIG_HOME or ~/.config everywhere
// else.
//
// macOS follows the XDG convention here rather than the ~/Library/Application
// Support that os.UserConfigDir answers, so that one dotfiles layout reaches
// the file on every Unix. A path with a space in it, behind a per-OS branch
// in the dotfiles, is a cost paid every time the settings move machine.
func Path() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", fmt.Errorf("locate the config directory: %w", err)
	}
	return filepath.Join(dir, "octoscope", "config.yaml"), nil
}

func configDir() (string, error) {
	if runtime.GOOS == "windows" {
		return os.UserConfigDir()
	}
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config"), nil
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
