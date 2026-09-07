// Command octoscope is a standalone terminal dashboard for GitHub.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jeandeaual/go-locale"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh/cli"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/app"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/usecase"
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
			"OCTOSCOPE_ICONS sets it permanently")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("octoscope " + version)
		return
	}

	osLocale, _ := locale.GetLocale() // an error here just means "unknown"
	i18n.SetLanguage(i18n.Resolve(*lang, osLocale))
	icon.Use(icon.Resolve(*icons))

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	// Whether the current directory has a repository is settled by the UI,
	// not here: answering it costs a gh subprocess, and waiting for one before
	// the first frame left the terminal blank for as long as it took.
	client := cli.New(dir, *repoFlag)
	uc := usecase.New(client)
	p := tea.NewProgram(app.New(uc, app.Options{HasRepo: *repoFlag != ""}))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
