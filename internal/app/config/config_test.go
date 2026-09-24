package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/config"
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

	path := write(t, "language: ja\nicons: nerd\nbackend: api\ndefault_tab: repos\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{Language: "ja", Icons: "nerd", Backend: "api", DefaultTab: "repos"}
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

	path := write(t, "language: ja\nfuture_field: is:open\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Language != "ja" {
		t.Errorf("Language = %q, want %q", got.Language, "ja")
	}
}

// The settings file names a tab, not a boolean: a boolean cannot say
// "search", and every tab added later would need another one.
func TestDefaultTabNamesTheTab(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"repos":    "repos",
		"  SEARCH": "search",
		"work":     "",
		"":         "",
		"nonsense": "",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			if got := (config.Config{DefaultTab: in}).DefaultTabName(); got != want {
				t.Errorf("DefaultTabName(%q) = %q, want %q", in, got, want)
			}
		})
	}
}

// A name the settings file does not know must not stop octoscope from
// starting: a typo in a setting is not a reason to lose GitHub.
func TestAnUnknownTabStartsOnTheDefault(t *testing.T) {
	t.Parallel()

	if got := (config.Config{DefaultTab: "detail"}).DefaultTabName(); got != "" {
		t.Errorf("DefaultTabName = %q, want the empty default", got)
	}
}

// Path is the one function that decides where the settings file lives on
// every platform; a broken join here silently makes every setting
// unreadable.
//
// The wanted path is spelled out rather than read back from os.UserConfigDir:
// a test that asked the same function the implementation asks would pass
// however the file moved. Windows has its own directory and its own
// environment variable, so the test has to say which it is setting.
func TestPathPutsTheFileUnderTheConfigDirectory(t *testing.T) {
	dir := t.TempDir()

	if runtime.GOOS == "windows" {
		t.Setenv("AppData", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
	}

	got, err := config.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, "octoscope", "config.yaml")
	if got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// Someone with no XDG_CONFIG_HOME set — which is most people on macOS — must
// still land in ~/.config, the one place a dotfiles repository can reach on
// every Unix.
func TestPathFallsBackToDotConfigWithoutXDG(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has %AppData%, not a home-relative fallback")
	}
	dir := t.TempDir()
	// Setting it empty rather than leaving it alone: the shell running the
	// tests may export one, and then this would not be testing the fallback.
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", dir)

	got, err := config.Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	want := filepath.Join(dir, ".config", "octoscope", "config.yaml")
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
