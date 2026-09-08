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
	Language   string `yaml:"language"`
	Icons      string `yaml:"icons"`
	DefaultTab string `yaml:"default_tab"`
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
