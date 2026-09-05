// Package app is the root model: it owns the tabs, hands each child its size,
// and shows the error screen.
package app

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/detail"
	"github.com/kukv/octoscope/internal/tui/repo"
	"github.com/kukv/octoscope/internal/tui/theme"
	"github.com/kukv/octoscope/internal/tui/work"
)

// Source is the union of what the child views need. Each view takes only its
// own slice of it.
type Source interface {
	work.Source
	repo.Source
	detail.Source
}

// Options carries what main determined before the UI started.
type Options struct {
	// HasRepo reports whether --repo named a target repository. Without the
	// flag the answer is not known yet: asking the GitHub layer to resolve
	// the working directory is a subprocess, and doing it before the UI
	// started left the terminal blank for as long as it took. The root asks
	// as soon as it has a size, and the Repos tab appears when the answer
	// arrives (spec 3.4).
	HasRepo bool
}

// repoLookupTimeout bounds the one call that decides whether the Repos tab
// exists. It is generous: the tab appearing late is a smaller problem than
// its never appearing on a slow network.
const repoLookupTimeout = 5 * time.Second

// repoResolvedMsg carries the answer to that lookup.
type repoResolvedMsg struct{ found bool }

// resolveRepo asks the GitHub layer to name the working directory's
// repository. Any failure means "there is none": a directory that is not a
// repository, one with nowhere to fetch from, and a network error all arrive
// the same way, and none of them gives the Repos tab anything to show.
func resolveRepo(src Source) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), repoLookupTimeout)
		defer cancel()
		name, err := src.RepoName(ctx)
		return repoResolvedMsg{found: err == nil && name != ""}
	}
}

type tabID int

const (
	tabWork tabID = iota
	tabRepos
)

type Model struct {
	src  Source
	opts Options

	width, height int
	tab           tabID

	work   work.Model
	repo   repo.Model
	detail detail.Model

	// started guards the first fetch, which waits for the first size rather
	// than happening in Init: work.Refresh hands back a model carrying the
	// cancel function of its request, and a value-receiver Init cannot keep it.
	started bool

	showingDetail bool
	errText       string
}

func New(src Source, opts Options) Model {
	return Model{
		src:  src,
		opts: opts,
		work: work.New(src),
		repo: repo.New(src),
	}
}

// Init asks the terminal for its background colour and nothing else: the
// first fetch runs from the first tea.WindowSizeMsg, which Bubble Tea sends at
// start-up. See the started field.
func (m Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		// The palette cannot assume a background, and this is the only place
		// that learns the real one (.claude/rules/tui.md).
		theme.SetDark(msg.IsDark())
		return m, nil
	case tea.WindowSizeMsg:
		return m.resize(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case repoResolvedMsg:
		if !msg.found {
			return m, nil
		}
		m.opts.HasRepo = true
		// broadcast skips the list until this point, so it never saw the
		// WindowSizeMsg that told the others how wide they are: an unsized
		// list clips nothing and runs off the terminal.
		m.repo, _ = m.repo.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.repo.Init()
	case work.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case repo.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case detail.ClosedMsg:
		m.showingDetail = false
		return m, nil
	case work.ErrorMsg:
		return m.fail(msg.Err)
	case repo.ErrorMsg:
		return m.fail(msg.Err)
	case detail.ErrorMsg:
		// The detail view keeps requests in flight after the user leaves it.
		// Their failures must not drag a closed view's error onto the screen.
		if !m.showingDetail {
			return m, nil
		}
		return m.fail(msg.Err)
	}
	return m.broadcast(msg)
}

// broadcast hands a message to every live child rather than only to the one on
// screen. A view the user left keeps requests in flight, and their answers
// must still land: a list whose refresh returned while the detail view was
// open would otherwise show a spinner forever.
func (m Model) broadcast(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	m.work, cmd = m.work.Update(msg)
	cmds = append(cmds, cmd)

	if m.opts.HasRepo {
		m.repo, cmd = m.repo.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.showingDetail {
		m.detail, cmd = m.detail.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) resize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width, m.height = msg.Width, msg.Height

	next, cmd := m.broadcast(msg)
	m = next.(Model)
	cmds := []tea.Cmd{cmd}

	if !m.started {
		m.started = true
		var fetch tea.Cmd
		m.work, fetch = m.work.Refresh()
		cmds = append(cmds, fetch)
		if m.opts.HasRepo {
			cmds = append(cmds, m.repo.Init())
		} else {
			cmds = append(cmds, resolveRepo(m.src))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) openDetail(ref gh.ItemRef) (tea.Model, tea.Cmd) {
	m.detail = detail.New(m.src, ref)
	m.showingDetail = true
	// The view is built after the terminal size is known, so it never sees the
	// WindowSizeMsg that told the others how wide they are.
	m.detail, _ = m.detail.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, m.detail.Init()
}

// fail moves to the error screen. Only the environment's own failures are
// translated; anything GitHub said is shown as it said it
// (.claude/rules/errors.md).
func (m Model) fail(err error) (tea.Model, tea.Cmd) {
	if errors.Is(err, gh.ErrGhNotFound) {
		m.errText = i18n.T("error.gh_not_found")
	} else {
		m.errText = err.Error()
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// ctrl+c comes before any delegation: the detail view swallows keys while
	// a comment is posting or a picker is applying, and a quit routed through
	// it would never arrive.
	if msg.String() == "ctrl+c" {
		return m, m.quit()
	}

	if m.errText != "" {
		if k := msg.String(); k == "q" || k == "esc" {
			return m, m.quit()
		}
		return m, nil
	}

	// q leaves the detail view rather than the app, so it is delegated too.
	if m.showingDetail {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q":
		return m, m.quit()
	case "1":
		m.tab = tabWork
		return m, nil
	case "2":
		if m.opts.HasRepo {
			m.tab = tabRepos
		}
		return m, nil
	}

	var cmd tea.Cmd
	if m.tab == tabWork {
		m.work, cmd = m.work.Update(msg)
	} else {
		m.repo, cmd = m.repo.Update(msg)
	}
	return m, cmd
}

// quit stops the Work board's fetch before leaving, so a cancelled gh
// subprocess does not outlive the program.
func (m Model) quit() tea.Cmd {
	m.work.Cancel()
	return tea.Quit
}
