// Command octoscope is a standalone terminal dashboard for GitHub.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jeandeaual/go-locale"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh/cli"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/app"
	"github.com/kukv/octoscope/internal/tui/icon"
)

// version is set by GoReleaser via -ldflags at release build time.
var version = "dev"

// repoLookupTimeout bounds the one gh call main makes before the UI starts.
const repoLookupTimeout = 5 * time.Second

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

	client := cli.New(dir, *repoFlag)
	p := tea.NewProgram(app.New(client, app.Options{
		HasRepo: hasRepo(client, *repoFlag),
	}))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// hasRepo reports whether a target repository is known. An explicit --repo
// settles it; otherwise we ask gh, which resolves the git remote of the
// working directory and fails when there is none.
func hasRepo(c *cli.Client, flagRepo string) bool {
	if flagRepo != "" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), repoLookupTimeout)
	defer cancel()
	_, err := c.RepoName(ctx)
	return err == nil
}
