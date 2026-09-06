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
	"github.com/kukv/octoscope/internal/tui/diff"
	"github.com/kukv/octoscope/internal/tui/repo"
	"github.com/kukv/octoscope/internal/tui/review"
	"github.com/kukv/octoscope/internal/tui/theme"
	"github.com/kukv/octoscope/internal/tui/work"
)

// Source is the union of what the child views need. Each view takes only its
// own slice of it.
type Source interface {
	work.Source
	repo.Source
	detail.Source
	diff.Source
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
// its never appearing on a slow network. `gh repo view` reaches the API, and
// a cold one has been measured at over six seconds.
const repoLookupTimeout = 20 * time.Second

// repoResolvedMsg carries the answer to that lookup. timedOut is kept apart
// from found because the two look identical on screen — no Repos tab — and
// only one of them is the truth about the directory.
type repoResolvedMsg struct {
	found    bool
	timedOut bool
}

// resolveRepo asks the GitHub layer to name the working directory's
// repository. A failure means "there is none" — a directory that is not a
// repository and one with nowhere to fetch from give the Repos tab nothing to
// show either — except a timeout, which is reported: silently dropping the tab
// because the network was slow reads as a bug in whatever else was changed.
func resolveRepo(src Source) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), repoLookupTimeout)
		defer cancel()
		name, err := src.RepoName(ctx)
		return resolved(ctx, name, err)
	}
}

// resolved reads one lookup's outcome. exec reports a killed subprocess as
// "signal: killed" rather than wrapping the deadline, so whether time ran out
// is a question for the context, not for the error.
func resolved(ctx context.Context, name string, err error) repoResolvedMsg {
	return repoResolvedMsg{
		found:    err == nil && name != "",
		timedOut: err != nil && ctx.Err() != nil,
	}
}

type tabID int

const (
	tabWork tabID = iota
	tabRepos
)

// overlay is a view drawn over the tabs. They stack: d from the detail view
// puts the diff on top of it, and esc takes it back off.
type overlay int

const (
	overlayDetail overlay = iota
	overlayDiff
)

type Model struct {
	src  Source
	opts Options

	width, height int
	tab           tabID

	// now is when the last message arrived. The tab row reports how old the
	// board's data is, and View may not read a clock of its own
	// (.claude/rules/tui.md), so the clock is read here.
	now time.Time

	work   work.Model
	repo   repo.Model
	detail detail.Model
	diff   diff.Model

	// started guards the first fetch, which waits for the first size rather
	// than happening in Init: work.Refresh hands back a model carrying the
	// cancel function of its request, and a value-receiver Init cannot keep it.
	started bool

	// stack holds the views drawn over the tabs, bottom first. Empty means
	// the tabs are showing. A bool could not tell "the diff over the detail
	// view" apart from "the diff on its own", and esc has to know which one
	// to go back to.
	stack   []overlay
	errText string

	// repoLookupTimedOut says the Repos tab is missing because the lookup ran
	// out of time, not because there is no repository here.
	repoLookupTimedOut bool
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
	m.now = time.Now()
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
			m.repoLookupTimedOut = msg.timedOut
			return m, nil
		}
		m.opts.HasRepo = true
		// broadcast skips the list until this point, so it never saw the
		// WindowSizeMsg that told the others how wide they are: an unsized
		// list clips nothing and runs off the terminal.
		m.repo, _ = m.repo.Update(tea.WindowSizeMsg{
			Width:  m.width,
			Height: max(m.height-tabRowHeight, 1),
		})
		return m, m.repo.Init()
	case work.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case repo.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case work.OpenDiffMsg:
		return m.openDiff(msg.Ref)
	case repo.OpenDiffMsg:
		return m.openDiff(msg.Ref)
	case detail.OpenDiffMsg:
		return m.openDiffOverDetail(msg.Ref)
	case detail.ClosedMsg:
		m.stack = m.pop(overlayDetail)
		return m, nil
	case diff.ClosedMsg:
		m.stack = m.pop(overlayDiff)
		return m, nil
	case work.ErrorMsg:
		return m.fail(msg.Err)
	case repo.ErrorMsg:
		return m.fail(msg.Err)
	case detail.ErrorMsg:
		// The detail view keeps requests in flight after the user leaves it.
		// Their failures must not drag a closed view's error onto the screen.
		if !m.has(overlayDetail) {
			return m, nil
		}
		return m.fail(msg.Err)
	case diff.ErrorMsg:
		// Same rule as detail.ErrorMsg: a request outlives the view that
		// started it, and its failure must not reach the error screen once
		// the diff is no longer on the stack.
		if !m.has(overlayDiff) {
			return m, nil
		}
		return m.fail(msg.Err)
	case review.SubmittedMsg:
		// detail and diff each refetch their own PR when a review goes out
		// (broadcast below reaches them); the board and the Repos list have
		// no popup of their own to notice from, so the root refreshes them
		// (spec 4.4.2).
		next, cmd := m.broadcast(msg)
		m = next.(Model)
		var workCmd tea.Cmd
		m.work, workCmd = m.work.Refresh()
		cmds := []tea.Cmd{cmd, workCmd}
		if m.opts.HasRepo {
			var repoCmd tea.Cmd
			m.repo, repoCmd = m.repo.Refresh()
			cmds = append(cmds, repoCmd)
		}
		return m, tea.Batch(cmds...)
	}
	return m.broadcast(msg)
}

// has reports whether o is anywhere on the stack, not only on top: the
// detail view keeps fetching while the diff is drawn over it.
func (m Model) has(o overlay) bool {
	for _, x := range m.stack {
		if x == o {
			return true
		}
	}
	return false
}

// top is the view on screen, if any.
func (m Model) top() (overlay, bool) {
	if len(m.stack) == 0 {
		return 0, false
	}
	return m.stack[len(m.stack)-1], true
}

// pop takes o off the stack, but only when it is the one on top. A
// ClosedMsg answers the view that sent it, and a second one queued before
// the first lands (two quick esc presses) must not pop twice.
func (m Model) pop(o overlay) []overlay {
	if top, ok := m.top(); !ok || top != o {
		return m.stack
	}
	return m.stack[:len(m.stack)-1]
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

	if m.has(overlayDetail) {
		m.detail, cmd = m.detail.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.has(overlayDiff) {
		m.diff, cmd = m.diff.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) resize(msg tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.width, m.height = msg.Width, msg.Height

	// A tab is told how much room is left under the tab row, not how big the
	// terminal is: it lays its own contents out to fit, and a tab that
	// measured the whole screen would push the last of them off the bottom.
	// A view on the stack is drawn from the top and gets the whole thing.
	next, cmd := m.broadcast(tea.WindowSizeMsg{
		Width:  msg.Width,
		Height: max(msg.Height-tabRowHeight, 1),
	})
	m = next.(Model)
	cmds := []tea.Cmd{cmd}
	if m.has(overlayDetail) {
		var detailCmd tea.Cmd
		m.detail, detailCmd = m.detail.Update(msg)
		cmds = append(cmds, detailCmd)
	}
	if m.has(overlayDiff) {
		var diffCmd tea.Cmd
		m.diff, diffCmd = m.diff.Update(msg)
		cmds = append(cmds, diffCmd)
	}

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
	m.stack = []overlay{overlayDetail}
	// The view is built after the terminal size is known, so it never sees the
	// WindowSizeMsg that told the others how wide they are.
	m.detail, _ = m.detail.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, m.detail.Init()
}

// openDiff shows the diff on its own, with the tabs underneath: the Work
// board and a Repos row have no detail view open when they ask for it.
func (m Model) openDiff(ref gh.ItemRef) (tea.Model, tea.Cmd) {
	m.stack = []overlay{overlayDiff}
	return m.startDiff(ref)
}

// openDiffOverDetail puts the diff on top of the detail view, so esc goes
// back to it rather than to the tabs.
func (m Model) openDiffOverDetail(ref gh.ItemRef) (tea.Model, tea.Cmd) {
	m.stack = append(m.stack, overlayDiff)
	return m.startDiff(ref)
}

func (m Model) startDiff(ref gh.ItemRef) (tea.Model, tea.Cmd) {
	m.diff = diff.New(m.src, ref)
	// The view is built after the terminal size is known, so it never sees the
	// WindowSizeMsg that told the others how wide they are.
	m.diff, _ = m.diff.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, m.diff.Init()
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
		switch msg.String() {
		case "q":
			return m, m.quit()
		case "esc":
			// Nothing to go back to at start-up (gh not on PATH, say): both
			// keys still quit. Once an overlay is on the stack, esc takes the
			// view that just failed off it and returns to whatever is
			// underneath, rather than costing the whole session over one
			// diff that would not load.
			if len(m.stack) == 0 {
				return m, m.quit()
			}
			m.errText = ""
			m.stack = m.stack[:len(m.stack)-1]
			return m, nil
		}
		return m, nil
	}

	// q leaves the view on top rather than the app, so it is delegated too.
	if top, ok := m.top(); ok {
		var cmd tea.Cmd
		switch top {
		case overlayDiff:
			m.diff, cmd = m.diff.Update(msg)
		default:
			m.detail, cmd = m.detail.Update(msg)
		}
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
