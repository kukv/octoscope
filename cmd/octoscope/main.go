// Command octoscope is a standalone terminal dashboard for GitHub.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/jeandeaual/go-locale"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/config"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/root"
	"github.com/kukv/octoscope/internal/app/usecase"
	"github.com/kukv/octoscope/internal/github/api"
	"github.com/kukv/octoscope/internal/i18n"
)

// version is set by GoReleaser via -ldflags at release build time.
var version = "dev"

func main() {
	repoFlag := flag.String("repo", "",
		"target repository as owner/name; defaults to the repository of the current directory")
	lang := flag.String("lang", "",
		"display language: en or ja; defaults to the operating system locale")
	icons := flag.String("icons", "",
		"glyph set: unicode (default), nerd for a Nerd Font patched font, or ascii; "+
			"OCTOSCOPE_ICONS or the settings file can set it permanently")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("octoscope " + version)
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

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Whether the current directory has a repository is settled by the UI,
	// not here: answering it costs a gh subprocess, and waiting for one before
	// the first frame left the terminal blank for as long as it took.
	ghClient, apiClient, err := chooseBackend(dir, *repoFlag, exec.LookPath, api.Token)
	if err != nil {
		// Printed before the program starts: once it is in the alt screen,
		// nothing written here survives the screen being cleared.
		fmt.Fprintln(os.Stderr, i18n.T("error.no_backend"))
		os.Exit(1)
	}

	// The store is built even when config.Path failed: it reports that
	// failure when something is saved, rather than saving nothing in silence.
	store := config.NewStore(path)
	var uc *usecase.Usecase
	if ghClient != nil {
		uc = usecase.New(ghClient, store)
	} else {
		uc = usecase.New(apiClient, store)
	}
	p := tea.NewProgram(root.New(uc, root.Options{
		Repo:         *repoFlag,
		Repositories: cfg.Repositories,
		SavedQueries: usecase.SavedQueriesFrom(cfg.SavedQueries),
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
