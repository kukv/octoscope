// Package app is the root model: it owns the tabs, hands each child its size,
// and shows the error screen.
package app

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/browser"
	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/checks"
	"github.com/kukv/octoscope/internal/tui/detail"
	"github.com/kukv/octoscope/internal/tui/diff"
	"github.com/kukv/octoscope/internal/tui/merge"
	"github.com/kukv/octoscope/internal/tui/repo"
	"github.com/kukv/octoscope/internal/tui/review"
	"github.com/kukv/octoscope/internal/tui/search"
	"github.com/kukv/octoscope/internal/tui/theme"
	"github.com/kukv/octoscope/internal/tui/work"
)

// repoNamer names the repository of the working directory. It is the root's
// alone: the tabs are told which repository they show.
type repoNamer interface {
	RepoName(ctx context.Context) (string, error)
}

// Source is the union of what the child views need. Each view takes only its
// own slice of it.
type Source interface {
	work.Source
	repo.Source
	detail.Source
	diff.Source
	checks.Source
	search.Source
	repoNamer
}

// Options carries what main determined before the UI started.
type Options struct {
	// Repo is what --repo named, if anything. Without the flag the
	// repository of the working directory is not known yet: asking the
	// GitHub layer is a subprocess, and doing it before the UI started left
	// the terminal blank for as long as it took. The root asks as soon as it
	// has a size, and the answer reaches the list then.
	Repo string

	// Repositories is the settings file's list, which the Repos tab shows
	// whether or not the working directory is a repository. internal/tui
	// cannot read internal/config, so the list travels here.
	Repositories []string

	// DefaultRepos says the settings file asks for the Repos tab at
	// start-up. It cannot be honoured until the current repository is known,
	// so it is remembered here and spent when the lookup answers. It is
	// weaker than --repo, which is a statement about this run.
	DefaultRepos bool

	// ConfigError is why the settings file could not be read, if it could
	// not. Empty means it was read, or was not there at all -- which is not
	// a failure. The run carries on with defaults either way, so the tab row
	// is the only place the user learns of it.
	ConfigError string
}

// repoLookupTimeout bounds the one call that decides which repository the
// list treats as the user's own. It is generous: the answer arriving late is
// a smaller problem than its never arriving on a slow network. `gh repo view`
// reaches the API, and a cold one has been measured at over six seconds.
const repoLookupTimeout = 20 * time.Second

// repoResolvedMsg carries the answer to that lookup. timedOut is kept apart
// from an empty name because the two look identical on screen — no current
// repository — and only one of them is the truth about the directory.
type repoResolvedMsg struct {
	name     string
	timedOut bool
}

// resolveRepo asks the GitHub layer to name the working directory's
// repository. A failure means "there is none" — a directory that is not a
// repository and one with nowhere to fetch from both leave the list without a
// current repository — except a timeout, which is reported: silently
// dropping it because the network was slow reads as a bug in whatever else
// was changed.
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
	if err != nil {
		return repoResolvedMsg{timedOut: ctx.Err() != nil}
	}
	return repoResolvedMsg{name: name}
}

type tabID int

const (
	tabWork tabID = iota
	tabRepos
	tabSearch
)

// overlay is a view drawn over the tabs. They stack: d from the detail view
// puts the diff on top of it, and esc takes it back off.
type overlay int

const (
	overlayDetail overlay = iota
	overlayDiff
	overlayChecks
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
	checks checks.Model
	search search.Model

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

	// errOverlay names which overlay's fetch produced errText, when any did
	// (errFromOverlay). A tab's failure is not tied to an overlay at all: it
	// can arrive while an overlay sits on top of the stack (submit a review
	// from the diff, the root refreshes the board, the board finds gh gone),
	// and esc must then clear the error without popping a view that never
	// failed.
	errOverlay     overlay
	errFromOverlay bool

	// repoLookupTimedOut says the current repository is unknown because the
	// lookup ran out of time, not because the working directory has none.
	repoLookupTimedOut bool

	// wantRepos holds the settings file's opening tab until the current
	// repository is known. It is cleared once spent, so a later answer to
	// the same lookup does not pull the user back off the tab they moved to.
	wantRepos bool
}

func New(src Source, opts Options) Model {
	m := Model{
		src:  src,
		opts: opts,
		work: work.New(src),
		repo: repo.New(src, repo.Options{
			Repositories: opts.Repositories,
			Current:      opts.Repo,
		}),
		search: search.New(src),
	}
	// Naming a repository on the command line is a statement about what the
	// user came to look at, so that is the tab they land on. A repository
	// found later, from the working directory, does not move them by itself
	// -- unless the settings file asked for the Repos tab, in which case
	// wantRepos spends that request when the lookup answers: see
	// repoResolved.
	if opts.Repo != "" {
		m.tab = tabRepos
	}
	m.wantRepos = opts.DefaultRepos
	return m
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
		return m.repoResolved(msg)
	case work.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case repo.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case search.OpenDetailMsg:
		return m.openDetail(msg.Ref)
	case work.OpenDiffMsg:
		return m.openDiff(msg.Ref)
	case repo.OpenDiffMsg:
		return m.openDiff(msg.Ref)
	case search.OpenDiffMsg:
		return m.openDiff(msg.Ref)
	case detail.OpenDiffMsg:
		return m.openDiffOverDetail(msg.Ref)
	case work.OpenChecksMsg:
		return m.openChecks(msg.Ref)
	case repo.OpenChecksMsg:
		return m.openChecks(msg.Ref)
	case detail.OpenChecksMsg:
		return m.openChecksOverDetail(msg.Ref)
	case detail.ClosedMsg:
		m.stack = m.pop(overlayDetail)
		return m, nil
	case diff.ClosedMsg:
		m.stack = m.pop(overlayDiff)
		return m, nil
	case checks.ClosedMsg:
		m.stack = m.pop(overlayChecks)
		return m, nil
	case work.FatalMsg:
		return m.fail(msg.Err)
	case repo.FatalMsg:
		return m.fail(msg.Err)
	case search.FatalMsg:
		return m.fail(msg.Err)
	case detail.ErrorMsg:
		return m.detailFailed(msg)
	case diff.ErrorMsg:
		return m.diffFailed(msg)
	case checks.ErrorMsg:
		return m.checksFailed(msg)
	case review.SubmittedMsg:
		return m.refreshLists(msg)
	case merge.MergedMsg:
		return m.refreshLists(msg)
	}
	return m.broadcast(msg)
}

func (m Model) repoResolved(msg repoResolvedMsg) (tea.Model, tea.Cmd) {
	m.repoLookupTimedOut = msg.timedOut
	// An empty name is passed on too, and so is a timeout: the list draws an
	// empty sidebar differently before the lookup has answered and after,
	// and only this message can tell it which it is looking at.
	var cmd tea.Cmd
	m.repo, cmd = m.repo.SetCurrent(msg.name)
	if msg.name == "" {
		return m, cmd
	}
	// The user stays where they are: the answer arrives seconds after the
	// board is already on screen -- unless the settings file asked for the
	// Repos tab, in which case wantRepos moves them there once.
	if m.wantRepos {
		m.wantRepos = false
		m.tab = tabRepos
	}
	return m, cmd
}

// The detail view keeps requests in flight after the user leaves it.
// Their failures must not drag a closed view's error onto the screen.
func (m Model) detailFailed(msg detail.ErrorMsg) (tea.Model, tea.Cmd) {
	if !m.has(overlayDetail) {
		return m, nil
	}
	return m.failOverlay(msg.Err, overlayDetail)
}

// Same rule as detail.ErrorMsg: a request outlives the view that
// started it, and its failure must not reach the error screen once
// the diff is no longer on the stack.
func (m Model) diffFailed(msg diff.ErrorMsg) (tea.Model, tea.Cmd) {
	if !m.has(overlayDiff) {
		return m, nil
	}
	return m.failOverlay(msg.Err, overlayDiff)
}

// Same rule as detail.ErrorMsg and diff.ErrorMsg: a request outlives the
// view that started it, and its failure must not reach the error screen
// once checks is no longer on the stack.
func (m Model) checksFailed(msg checks.ErrorMsg) (tea.Model, tea.Cmd) {
	if !m.has(overlayChecks) {
		return m, nil
	}
	return m.failOverlay(msg.Err, overlayChecks)
}

// refreshLists carries a review submission or a merge to the views that are
// open and refetches the board and the Repos list, which have nothing of
// their own to notice from. A review submission is refetched by the detail
// and diff views; of a merge only the detail view takes notice, closing when
// the pull request was merged and letting the popup refetch when it only
// joined or left the auto-merge queue.
func (m Model) refreshLists(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.broadcast(msg)
	m = next.(Model)
	var workCmd tea.Cmd
	m.work, workCmd = m.work.Refresh()
	var repoCmd tea.Cmd
	m.repo, repoCmd = m.repo.Refresh()
	return m, tea.Batch(cmd, workCmd, repoCmd)
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

	m.repo, cmd = m.repo.Update(msg)
	cmds = append(cmds, cmd)

	m.search, cmd = m.search.Update(msg)
	cmds = append(cmds, cmd)

	if m.has(overlayDetail) {
		m.detail, cmd = m.detail.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.has(overlayDiff) {
		m.diff, cmd = m.diff.Update(msg)
		cmds = append(cmds, cmd)
	}

	if m.has(overlayChecks) {
		m.checks, cmd = m.checks.Update(msg)
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
	if m.has(overlayChecks) {
		var checksCmd tea.Cmd
		m.checks, checksCmd = m.checks.Update(msg)
		cmds = append(cmds, checksCmd)
	}

	if !m.started {
		m.started = true
		var fetch tea.Cmd
		m.work, fetch = m.work.Refresh()
		cmds = append(cmds, fetch, m.repo.Init(), m.search.Init())
		if m.opts.Repo == "" {
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

// openChecks shows the checks on their own, with the tabs underneath: the
// Work board and a Repos row have no detail view open when they ask for it.
func (m Model) openChecks(ref gh.ItemRef) (tea.Model, tea.Cmd) {
	m.stack = []overlay{overlayChecks}
	return m.startChecks(ref)
}

// openChecksOverDetail puts checks on top of the detail view, so esc goes
// back to it rather than to the tabs.
func (m Model) openChecksOverDetail(ref gh.ItemRef) (tea.Model, tea.Cmd) {
	m.stack = append(m.stack, overlayChecks)
	return m.startChecks(ref)
}

func (m Model) startChecks(ref gh.ItemRef) (tea.Model, tea.Cmd) {
	m.checks = checks.New(m.src, ref)
	// The view is built after the terminal size is known, so it never sees the
	// WindowSizeMsg that told the others how wide they are.
	m.checks, _ = m.checks.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	return m, m.checks.Init()
}

// fail moves to the error screen for a failure that is not tied to any one
// overlay: a tab has found that the user must act before anything can work
// (gh.IsFatal), everything else being a line on the tab itself. esc on this
// error must never pop the stack: whatever overlay is on top of it, if any,
// did not fail.
func (m Model) fail(err error) (tea.Model, tea.Cmd) {
	m.errFromOverlay = false
	return m.showError(err)
}

// failOverlay moves to the error screen for a failure that belongs to one
// overlay (the detail view or the diff). esc pops the stack only when that
// overlay is still the one on top of it (see handleKey): a stale failure
// from a view the user is no longer looking at must not discard whatever is
// on top now.
func (m Model) failOverlay(err error, o overlay) (tea.Model, tea.Cmd) {
	m.errFromOverlay = true
	m.errOverlay = o
	return m.showError(err)
}

// showError renders err onto the error screen. Only the environment's own
// failures are translated; anything GitHub said is shown as it said it
// (.claude/rules/errors.md).
func (m Model) showError(err error) (tea.Model, tea.Cmd) {
	var noBrowser *browser.NoneError
	switch {
	case errors.Is(err, gh.ErrGhNotFound):
		m.errText = i18n.T("error.gh_not_found")
	case errors.Is(err, gh.ErrUnauthenticated):
		m.errText = i18n.T("error.unauthenticated")
	case errors.As(err, &noBrowser):
		m.errText = i18n.Tf("error.no_browser", map[string]any{"URL": noBrowser.URL})
	default:
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
			// Only pop when the view that failed is the one on top: a tab's
			// failure (errFromOverlay false) never belongs to an overlay,
			// and a stale overlay failure can arrive after
			// another overlay was pushed on top of it. Either way the view
			// on screen did not fail and must stay put.
			if m.errFromOverlay {
				if top, ok := m.top(); ok && top == m.errOverlay {
					m.stack = m.stack[:len(m.stack)-1]
				}
			}
			m.errFromOverlay = false
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
		case overlayChecks:
			m.checks, cmd = m.checks.Update(msg)
		default:
			m.detail, cmd = m.detail.Update(msg)
		}
		return m, cmd
	}

	var cmd tea.Cmd

	// A tab with a field open takes every key, the same way an overlay does.
	// Without this, typing a repository name or a search query with a q in
	// it would quit, or a digit in it would jump tabs.
	if m.capturing() {
		switch m.tab {
		case tabRepos:
			m.repo, cmd = m.repo.Update(msg)
		case tabSearch:
			m.search, cmd = m.search.Update(msg)
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
		m.tab = tabRepos
		return m, nil
	case "3":
		m.tab = tabSearch
		return m, nil
	}

	switch m.tab {
	case tabWork:
		m.work, cmd = m.work.Update(msg)
	case tabRepos:
		m.repo, cmd = m.repo.Update(msg)
	case tabSearch:
		m.search, cmd = m.search.Update(msg)
	}
	return m, cmd
}

// capturing says the active tab has a field open that must see every key,
// the same way an overlay does.
func (m Model) capturing() bool {
	switch m.tab {
	case tabRepos:
		return m.repo.Capturing()
	case tabSearch:
		return m.search.Capturing()
	}
	return false
}

// quit stops the Work board's fetch before leaving, so a cancelled gh
// subprocess does not outlive the program.
func (m Model) quit() tea.Cmd {
	m.work.Cancel()
	return tea.Quit
}
