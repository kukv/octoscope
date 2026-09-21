// Package datasource reads and writes the application's own data -- the
// repositories the Repos tab lists and the Search tab's saved queries --
// through whatever store it is pointed at. Today that store is the settings
// file, whose shape internal/app/config owns; this package translates
// between that shape and the domain's.
package datasource

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	yaml "go.yaml.in/yaml/v3"

	"github.com/kukv/octoscope/internal/app/config"
	"github.com/kukv/octoscope/internal/app/domain"
)

type Store struct{ path string }

// NewStore returns a store for the settings file at path. An empty path is
// allowed: main builds a store even when it could not locate the config
// directory, so that the failure surfaces on the first save rather than as a
// save that silently does nothing.
func NewStore(path string) *Store { return &Store{path: path} }

// Repositories is the list the Repos tab shows, in the order it shows them.
// An empty path reads as nothing saved, the same as a first run; only the
// save side reports it as an error.
func (s *Store) Repositories() ([]string, error) {
	c, err := config.Load(s.path)
	if err != nil {
		return nil, err
	}
	return c.Repositories, nil
}

// SavedQueries is the Search tab's saved queries, in the order the user
// saved them. An empty path reads as nothing saved, the same as a first
// run; only the save side reports it as an error.
func (s *Store) SavedQueries() ([]domain.SavedQuery, error) {
	c, err := config.Load(s.path)
	if err != nil {
		return nil, err
	}
	out := make([]domain.SavedQuery, len(c.SavedQueries))
	for i, q := range c.SavedQueries {
		out[i] = domain.SavedQuery{Name: q.Name, Query: q.Query}
	}
	return out, nil
}

// SaveRepositories replaces the repository list and leaves every other
// setting as it was. A file that cannot be parsed is not written at all: a
// list is not worth flattening the rest of someone's settings for.
func (s *Store) SaveRepositories(repos []string) error {
	c, err := s.load()
	if err != nil {
		return err
	}
	c.Repositories = repos
	return s.save(c)
}

// SaveQueries replaces the saved queries and leaves every other setting as
// it was, for the same reason SaveRepositories does.
func (s *Store) SaveQueries(queries []domain.SavedQuery) error {
	c, err := s.load()
	if err != nil {
		return err
	}
	entries := make([]config.SavedQueryEntry, len(queries))
	for i, q := range queries {
		entries[i] = config.SavedQueryEntry{Name: q.Name, Query: q.Query}
	}
	c.SavedQueries = entries
	return s.save(c)
}

// load is the read every write starts from, so that a write replaces one
// setting rather than the file.
func (s *Store) load() (config.Config, error) {
	if s.path == "" {
		return config.Config{}, errors.New("no settings file to write: the config directory could not be located")
	}
	return config.Load(s.path)
}

// save writes through a temporary file in the same directory so that an
// interrupted write cannot leave a half-written settings file behind.
func (s *Store) save(c config.Config) error {
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
