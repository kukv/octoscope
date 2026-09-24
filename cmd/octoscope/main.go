// Command octoscope is a standalone terminal dashboard for GitHub.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"runtime/debug"

	"github.com/jeandeaual/go-locale"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/adapter/datasource"
	"github.com/kukv/octoscope/internal/app/adapter/gateway/gh"
	"github.com/kukv/octoscope/internal/app/config"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/root"
	"github.com/kukv/octoscope/internal/app/usecase"
	"github.com/kukv/octoscope/internal/github/api"
	"github.com/kukv/octoscope/internal/i18n"
)

// version is what --version prints. GoReleaser sets it with -ldflags at
// release build time; a build that did not go through GoReleaser leaves it
// empty, and the module's own version is read back out of the binary
// instead (see resolveVersion).
var version string

// resolveVersion picks what to call this build. injected is what -ldflags
// put in, empty when nothing did. module is what the Go toolchain recorded
// as the main module's version: a real version for a binary from
// `go install module@version`, and "(devel)" for one built from a source
// tree, which says nothing a user wants to read. Neither being any use
// leaves "dev".
func resolveVersion(injected, module string) string {
	if injected != "" {
		return injected
	}
	if module != "" && module != "(devel)" {
		return module
	}
	return "dev"
}

func main() {
	repoFlag := flag.String("repo", "",
		"target repository as owner/name; defaults to the repository of the current directory")
	lang := flag.String("lang", "",
		"display language: en or ja; defaults to the operating system locale")
	icons := flag.String("icons", "",
		"glyph set: unicode (default), nerd for a Nerd Font patched font, or ascii; "+
			"OCTOSCOPE_ICONS or the settings file can set it permanently")
	backendFlag := flag.String("backend", "",
		"how to reach GitHub: auto (gh if it is on PATH, else a token; the default), gh, "+
			"or api (GH_TOKEN or GITHUB_TOKEN, without gh); "+
			"OCTOSCOPE_BACKEND or the settings file can set it permanently")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		module := ""
		if info, ok := debug.ReadBuildInfo(); ok {
			module = info.Main.Version
		}
		fmt.Println("octoscope " + resolveVersion(version, module))
		return
	}

	var cfg config.Config
	var configErr string
	path, err := config.Path()
	if err != nil {
		configErr = err.Error()
	} else if cfg, err = config.Load(path); err != nil {
		configErr = err.Error()
	}

	osLocale, _ := locale.GetLocale() // an error here just means "unknown"
	i18n.SetLanguage(i18n.Resolve(*lang, cfg.Language, osLocale))
	icon.Use(icon.Resolve(*icons, cfg.Icons))

	// Resolved after the language, so that a bad value is reported in it.
	mode, err := resolveBackend(*backendFlag, os.Getenv(backendEnvVar), cfg.Backend)
	var unknown *unknownBackendError
	if errors.As(err, &unknown) {
		fmt.Fprintln(os.Stderr, i18n.Tf("error.unknown_backend",
			map[string]any{"Source": unknown.Source, "Value": unknown.Value}))
		os.Exit(2)
	}

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Whether the current directory has a repository is settled by the UI,
	// not here: answering it costs a gh subprocess, and waiting for one before
	// the first frame left the terminal blank for as long as it took.
	ghClient, apiClient, err := chooseBackend(dir, *repoFlag, mode, exec.LookPath, api.Token)
	if err != nil {
		// Printed before the program starts: once it is in the alt screen,
		// nothing written here survives the screen being cleared.
		fmt.Fprintln(os.Stderr, i18n.T(backendFailure(mode)))
		os.Exit(1)
	}

	// The store is built even when config.Path failed: it reports that
	// failure when something is saved, rather than saving nothing in silence.
	store := datasource.NewStore(path)
	var uc *usecase.Usecase
	if ghClient != nil {
		uc = usecase.New(gh.New(ghClient), store)
	} else {
		uc = usecase.New(gh.New(apiClient), store)
	}

	// The lists come from the store rather than out of cfg: the file's shape
	// is config's business, and what the application keeps in it is the
	// store's. A read failure here is the same one config.Load already put in
	// configErr, so it is not reported a second time; the run carries on with
	// nothing saved, which is what a first run does anyway.
	repos, _ := store.Repositories()
	queries, _ := store.SavedQueries()

	p := tea.NewProgram(root.New(uc, root.Options{
		Repo:         *repoFlag,
		Repositories: repos,
		SavedQueries: queries,
		DefaultTab:   cfg.DefaultTabName(),
		ConfigError:  configErr,
	}))
	_, runErr := p.Run()
	// Printed only now: p.Run put the terminal in the alt screen, and
	// anything written to stderr before that is gone once it clears.
	if configErr != "" {
		fmt.Fprintln(os.Stderr, i18n.T("error.config_unreadable"))
		fmt.Fprintln(os.Stderr, configErr)
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(1)
	}
}
