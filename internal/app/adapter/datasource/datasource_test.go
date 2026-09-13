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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	queries, err := datasource.NewStore(path).SavedQueries()
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 || queries[0].Name != "mine" || queries[0].Query != "is:open" {
		t.Errorf("saved queries = %v, want [{mine is:open}]", queries)
	}
}

func TestSavingRepositoriesLeavesTheQueriesAlone(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	if err := datasource.NewStore("").SaveRepositories([]string{"kukv/octoscope"}); err == nil {
		t.Error("SaveRepositories() with no path returned no error")
	}
	if err := datasource.NewStore("").SaveQueries(nil); err == nil {
		t.Error("SaveQueries() with no path returned no error")
	}
}

// TestSaveRepositoriesKeepsTheOtherSettings is the whole reason SaveRepositories
// reads before it writes: the sidebar knows only the repository list, and
// writing a Config built from that alone would drop everything else in the
// file.
func TestSaveRepositoriesKeepsTheOtherSettings(t *testing.T) {
	t.Parallel()
	path := write(t, "language: ja\nicons: nerd\ndefault_tab: repos\n")
	if err := datasource.NewStore(path).SaveRepositories([]string{"kukv/octoscope"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Language != "ja" || got.Icons != "nerd" || got.DefaultTab != "repos" {
		t.Errorf("save dropped the other settings: %+v", got)
	}
	if len(got.Repositories) != 1 || got.Repositories[0] != "kukv/octoscope" {
		t.Errorf("Repositories = %v, want [kukv/octoscope]", got.Repositories)
	}
}

// TestSaveRepositoriesRefusesAFileItCannotParse guards the worst outcome
// this feature can have: one keypress flattening a settings file whose YAML
// the user is in the middle of hand-editing.
func TestSaveRepositoriesRefusesAFileItCannotParse(t *testing.T) {
	t.Parallel()
	const broken = "language: [ja\n"
	path := write(t, broken)
	if err := datasource.NewStore(path).SaveRepositories([]string{"kukv/octoscope"}); err == nil {
		t.Fatal("SaveRepositories overwrote a file it could not parse")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != broken {
		t.Errorf("file = %q, want it untouched %q", raw, broken)
	}
}

// TestSaveQueriesRefusesAFileItCannotParse is SaveRepositories' guard above,
// for the other write path.
func TestSaveQueriesRefusesAFileItCannotParse(t *testing.T) {
	t.Parallel()
	const broken = "language: [ja\n"
	path := write(t, broken)
	if err := datasource.NewStore(path).SaveQueries(nil); err == nil {
		t.Fatal("SaveQueries overwrote a file it could not parse")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != broken {
		t.Errorf("file = %q, want it untouched %q", raw, broken)
	}
}

// TestSaveRepositoriesCreatesTheFileAndItsDirectory covers the first run:
// nothing under the OS config directory exists yet.
func TestSaveRepositoriesCreatesTheFileAndItsDirectory(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "octoscope", "config.yaml")
	if err := datasource.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Repositories) != 1 || got.Repositories[0] != "kukv/koto" {
		t.Errorf("Repositories = %v, want [kukv/koto]", got.Repositories)
	}
}

// TestSaveRepositoriesLeavesNoTempBehind is what temp+rename is for: a
// half-written file must never be the one Load reads, and a successful save
// must not litter the config directory either.
func TestSaveRepositoriesLeavesNoTempBehind(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := datasource.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.yaml" {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("directory holds %v, want only config.yaml", names)
	}
}
