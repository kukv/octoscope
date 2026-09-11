package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/config"
)

// A missing file is the default, not a failure: octoscope must run for
// someone who has never written a config.
func TestLoadTreatsAMissingFileAsDefaults(t *testing.T) {
	t.Parallel()

	got, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load of a missing file: %v", err)
	}
	if !reflect.DeepEqual(got, config.Config{}) {
		t.Errorf("Load of a missing file = %+v, want the zero Config", got)
	}
}

func TestLoadReadsEveryField(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\nicons: nerd\ndefault_tab: repos\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{Language: "ja", Icons: "nerd", DefaultTab: "repos"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

// A broken config must not keep the user from reaching GitHub: Load reports
// the failure, and the caller carries on with defaults.
func TestLoadReportsBrokenYAMLAndStillReturnsDefaults(t *testing.T) {
	t.Parallel()

	path := write(t, "language: [ja\n")

	got, err := config.Load(path)
	if err == nil {
		t.Fatal("Load of broken YAML returned no error")
	}
	if !reflect.DeepEqual(got, config.Config{}) {
		t.Errorf("Load of broken YAML = %+v, want the zero Config", got)
	}
}

// Keys this version does not know belong to a later one; they must not stop
// it from reading the keys it does know.
func TestLoadIgnoresKeysItDoesNotKnow(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\nsaved_queries:\n  - is:open\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Language != "ja" {
		t.Errorf("Language = %q, want %q", got.Language, "ja")
	}
}

// default_tab is as tolerant of case and surrounding whitespace as icons and
// language are; a user typing "Repos" must not silently land on Work.
func TestWantsReposIgnoresCaseAndSurroundingWhitespace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		defaultTab string
		want       bool
	}{
		{"empty", "", false},
		{"exact", "repos", true},
		{"mixed case", "Repos", true},
		{"surrounding whitespace", " repos ", true},
		{"unrelated value", "work", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := config.Config{DefaultTab: tt.defaultTab}.WantsRepos()
			if got != tt.want {
				t.Errorf("Config{DefaultTab: %q}.WantsRepos() = %v, want %v", tt.defaultTab, got, tt.want)
			}
		})
	}
}

// Path is the one function that decides where the settings file lives on
// every platform; a broken join here silently makes every setting
// unreadable.
func TestPathPutsTheFileUnderTheConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	got, err := config.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, "octoscope", "config.yaml")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// The Repos tab's list is the reason the settings file exists; it must
// survive a round trip through the parser in the order the user wrote it.
func TestLoadReadsTheRepositoryList(t *testing.T) {
	t.Parallel()

	path := write(t, "repositories:\n  - kukv/octoscope\n  - cli/cli\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"kukv/octoscope", "cli/cli"}
	if !slices.Equal(got.Repositories, want) {
		t.Errorf("Repositories = %v, want %v", got.Repositories, want)
	}
}

// A settings file with no list at all is the common case on a first run.
func TestLoadLeavesTheRepositoryListEmptyWhenTheFileHasNone(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Repositories) != 0 {
		t.Errorf("Repositories = %v, want none", got.Repositories)
	}
}

func write(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// TestSaveRepositoriesKeepsTheOtherSettings is the whole reason Save reads
// before it writes: the sidebar knows only the repository list, and writing
// a Config built from that alone would drop everything else in the file.
func TestSaveRepositoriesKeepsTheOtherSettings(t *testing.T) {
	t.Parallel()

	path := write(t, "language: ja\nicons: nerd\ndefault_tab: repos\n")
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/octoscope"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Language != "ja" || got.Icons != "nerd" || got.DefaultTab != "repos" {
		t.Errorf("save dropped the other settings: %+v", got)
	}
	if !slices.Equal(got.Repositories, []string{"kukv/octoscope"}) {
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
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/octoscope"}); err == nil {
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

// TestSaveRepositoriesCreatesTheFileAndItsDirectory covers the first run:
// nothing under the OS config directory exists yet.
func TestSaveRepositoriesCreatesTheFileAndItsDirectory(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "octoscope", "config.yaml")
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !slices.Equal(got.Repositories, []string{"kukv/koto"}) {
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
	if err := config.NewStore(path).SaveRepositories([]string{"kukv/koto"}); err != nil {
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

// TestSaveRepositoriesWithNoPathFails: main builds a store even when it
// could not locate the config directory, so the failure has to surface at
// the save rather than as a silent no-op.
func TestSaveRepositoriesWithNoPathFails(t *testing.T) {
	t.Parallel()

	if err := config.NewStore("").SaveRepositories([]string{"kukv/koto"}); err == nil {
		t.Error("saving to an empty path reported success")
	}
}
