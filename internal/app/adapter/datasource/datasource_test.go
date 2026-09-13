package datasource_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kukv/octoscope/internal/app/adapter/datasource"
	"github.com/kukv/octoscope/internal/app/config"
	"github.com/kukv/octoscope/internal/app/domain"
)

// write puts raw into a settings file inside a temporary directory and
// returns its path.
func write(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSavedQueriesComeBackAsDomainValues(t *testing.T) {
	path := write(t, "saved_queries:\n  - name: mine\n    query: is:open author:@me\n")
	got, err := datasource.NewStore(path).SavedQueries()
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.SavedQuery{{Name: "mine", Query: "is:open author:@me"}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("SavedQueries() = %v, want %v", got, want)
	}
}

func TestRepositoriesComeBackInFileOrder(t *testing.T) {
	path := write(t, "repositories:\n  - kukv/octoscope\n  - kukv/koto\n")
	got, err := datasource.NewStore(path).Repositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "kukv/octoscope" || got[1] != "kukv/koto" {
		t.Errorf("Repositories() = %v, want [kukv/octoscope kukv/koto]", got)
	}
}

// A settings file that is not there is not a failure: a first run has none.
func TestAMissingFileReadsAsNothingSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	s := datasource.NewStore(path)
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories() on a missing file: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("Repositories() = %v, want none", repos)
	}
	queries, err := s.SavedQueries()
	if err != nil {
		t.Fatalf("SavedQueries() on a missing file: %v", err)
	}
	if len(queries) != 0 {
		t.Errorf("SavedQueries() = %v, want none", queries)
	}
}

// This is the test the split exists for: two packages now touch one file,
// and what one writes the other has to be able to read.
func TestSavingQueriesLeavesTheStartupSettingsAlone(t *testing.T) {
	path := write(t, "language: ja\nicons: nerdfont\ndefault_tab: search\nrepositories:\n  - kukv/octoscope\n")
	if err := datasource.NewStore(path).SaveQueries([]domain.SavedQuery{{Name: "mine", Query: "is:open"}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Language != "ja" {
		t.Errorf("language = %q, want ja", cfg.Language)
	}
	if cfg.Icons != "nerdfont" {
		t.Errorf("icons = %q, want nerdfont", cfg.Icons)
	}
	if cfg.DefaultTabName() != "search" {
		t.Errorf("default tab = %q, want search", cfg.DefaultTabName())
	}
	repos, err := datasource.NewStore(path).Repositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0] != "kukv/octoscope" {
		t.Errorf("repositories = %v, want [kukv/octoscope]", repos)
	}
}

func TestSavingRepositoriesLeavesTheQueriesAlone(t *testing.T) {
	path := write(t, "saved_queries:\n  - name: mine\n    query: is:open\n")
	if err := datasource.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatal(err)
	}
	queries, err := datasource.NewStore(path).SavedQueries()
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0].Name != "mine" {
		t.Errorf("saved queries = %v, want the one that was there", queries)
	}
}

// main builds a store even when the config directory could not be located,
// so that the failure surfaces on the first save rather than as a save that
// silently does nothing.
func TestAStoreWithNoPathReportsItRatherThanSavingNothing(t *testing.T) {
	if err := datasource.NewStore("").SaveRepositories([]string{"kukv/octoscope"}); err == nil {
		t.Error("SaveRepositories() with no path returned no error")
	}
	if err := datasource.NewStore("").SaveQueries(nil); err == nil {
		t.Error("SaveQueries() with no path returned no error")
	}
}
