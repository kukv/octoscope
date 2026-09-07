// Package checks shows one pull request's checks: what ran down the left,
// the log of the selected one on the right.
package checks

import (
	"context"
	"sort"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// Source is what the checks view needs. repo is "owner/repo"; the empty
// string targets the workspace repository.
type Source interface {
	PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)
	JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error
	OpenWeb(url string) error
}

// ClosedMsg tells the parent the user left the checks view.
type ClosedMsg struct{}

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type checksMsg struct {
	ref    gh.ItemRef
	checks gh.Checks
}

type errMsg struct {
	ref gh.ItemRef
	err error
}

// pane is which side of the view the cursor acts on.
type pane uint8

const (
	paneList pane = iota
	paneLog
)

type Model struct {
	src Source
	ref gh.ItemRef

	width, height int

	loading bool
	spin    spinner.Model

	checks gh.Checks
	// order is the checks as drawn: failing workflows first, and the checks
	// of one workflow together. It is rebuilt when the checks arrive rather
	// than on every draw, because View may do no work of its own.
	order []gh.CheckRun
	row   int
	top   int

	pane pane

	mode mode

	// rerunPhase, rerunScope, rerunRunID and rerunWorkflow are the rerun
	// popup's own state: which run it targets (captured at R-press time,
	// see startRerun), which scope is picked, and whether a send is in
	// flight.
	rerunPhase    rerunPhase
	rerunScope    gh.RerunScope
	rerunRunID    int64
	rerunWorkflow string

	// log is the lines of the currently open job, failed steps only unless
	// failedOnly was toggled off. logJob is the id of the check that log
	// belongs to (or is being fetched for); an answer is kept only while the
	// cursor is still on the job it was asked for.
	log        []gh.LogLine
	logJob     int64
	logRow     int
	hscroll    int
	failedOnly bool
	logPhase   logPhase

	declined string
	// errText is the last log fetch's failure. The checks list has its own
	// escalation path (errMsg -> ErrorMsg): the list may still be readable
	// with no log open, so this stays a footer line instead.
	errText string
}

// New builds the view for one pull request's checks.
func New(src Source, ref gh.ItemRef) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{src: src, ref: ref, loading: true, spin: s, failedOnly: true}
}

// Init starts the fetch.
func (m Model) Init() tea.Cmd { return tea.Batch(m.spin.Tick, m.fetch()) }

func (m Model) fetch() tea.Cmd {
	src, ref := m.src, m.ref
	return func() tea.Msg {
		c, err := src.PRChecks(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return errMsg{ref: ref, err: err}
		}
		return checksMsg{ref: ref, checks: c}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg), nil
	case checksMsg:
		return m.checksArrived(msg), nil
	case errMsg:
		return m.fetchFailed(msg)
	case logMsg:
		return m.logArrived(msg), nil
	case logErrMsg:
		return m.logFailed(msg), nil
	case rerunDoneMsg:
		return m.rerunDone(msg), nil
	case rerunErrMsg:
		return m.rerunFailed(msg), nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		return m.tick(msg)
	}
	return m, nil
}

func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	return m.boundHscroll()
}

func (m Model) checksArrived(msg checksMsg) Model {
	// The request for the pull request the user just left is still in
	// flight; its answer must not replace this one's.
	if msg.ref != m.ref {
		return m
	}
	m.loading = false
	m.checks = msg.checks
	m.order = arrange(msg.checks.Runs)
	m.row = 0
	m.top = 0
	m.declined = ""
	m = m.clearLog()
	return m
}

func (m Model) fetchFailed(msg errMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.loading = false
	return m, func() tea.Msg { return ErrorMsg{Err: msg.err} }
}

func (m Model) tick(msg spinner.TickMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.mode == modeRerun {
		return m.handleRerunKey(msg)
	}
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "r":
		m.loading = true
		m.declined = ""
		m.errText = ""
		return m, tea.Batch(m.spin.Tick, m.fetch())
	case "R":
		return m.startRerun(), nil
	case "j", "down":
		return m.moveRow(1), nil
	case "k", "up":
		return m.moveRow(-1), nil
	case "h":
		return m.moveHscroll(-1), nil
	case "l":
		return m.moveHscroll(1), nil
	case "enter":
		return m.startLog(m.failedOnly)
	case "L":
		return m.startLog(!m.failedOnly)
	case "o":
		return m.openSelected()
	}
	return m, nil
}

func (m Model) moveRow(delta int) Model {
	if m.pane == paneLog {
		m.logRow = clamp(m.logRow+delta, len(m.logRows())-1)
		return m.boundHscroll()
	}
	before := m.row
	m.row = clamp(m.row+delta, len(m.order)-1)
	m.declined = ""
	// The log pane says nothing about whose log it holds, so one left under
	// another check would read as that check's.
	if m.row != before {
		m = m.clearLog()
	}
	return m.follow()
}

// selectedJobID is the job id of the check under the cursor, 0 if the
// cursor is on nothing (the list is empty or has not arrived yet).
func (m Model) selectedJobID() int64 {
	if m.row < 0 || m.row >= len(m.order) {
		return 0
	}
	return m.order[m.row].JobID
}

// openSelected opens the selected check's own page: detailsUrl for a check
// run, targetUrl for a StatusContext. A check that reports neither is said
// so at footer level rather than swallowed. A failure to open escalates the
// same way a failed checks fetch does -- there is nothing sensible left on
// screen once the browser could not be opened.
func (m Model) openSelected() (Model, tea.Cmd) {
	if len(m.order) == 0 {
		m.declined = i18n.T("checks.none")
		return m, nil
	}
	url := m.order[m.row].URL
	if url == "" {
		m.declined = i18n.T("checks.decline_no_url")
		return m, nil
	}
	m.declined = ""
	src, ref := m.src, m.ref
	return m, func() tea.Msg {
		if err := src.OpenWeb(url); err != nil {
			return errMsg{ref: ref, err: err}
		}
		return nil
	}
}

// follow scrolls the window so the cursor stays on it. top indexes the drawn
// lines, which count the workflow headings as well as the checks, so it is
// the cursor's line and not its row that has to stay inside the window.
func (m Model) follow() Model {
	h := m.paneHeight()
	line := m.cursorLine()
	// The first check of a group is only readable as one of that group with
	// the heading over it on screen, so it is the heading's line the window
	// stops at.
	top := line
	if m.row < len(m.order) && m.hasHeading(m.row) {
		top--
	}
	if top < m.top {
		m.top = top
	}
	if line >= m.top+h {
		m.top = line - h + 1
	}
	m.top = max(m.top, 0)
	return m
}

// arrange puts the failing workflows at the top and keeps each workflow's
// checks together. A check with no workflow behind it belongs to no group and
// sorts after all of them, the StatusContext last of those: it is the one the
// view can do the least with.
func arrange(runs []gh.CheckRun) []gh.CheckRun {
	worst := map[string]int{}
	first := map[string]int{}
	for i, r := range runs {
		if !hasWorkflow(r) {
			continue
		}
		worst[r.Workflow] = max(worst[r.Workflow], rank(r.State))
		if _, seen := first[r.Workflow]; !seen {
			first[r.Workflow] = i
		}
	}
	out := append([]gh.CheckRun(nil), runs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Kind == gh.CheckKindStatus) != (b.Kind == gh.CheckKindStatus) {
			return b.Kind == gh.CheckKindStatus
		}
		// Ranking a check with no workflow against the workflows would let
		// one such check's state carry every other one along with it: they
		// have no group of their own to be ranked as.
		if hasWorkflow(a) != hasWorkflow(b) {
			return hasWorkflow(a)
		}
		if !hasWorkflow(a) || a.Workflow == b.Workflow {
			return false
		}
		if worst[a.Workflow] != worst[b.Workflow] {
			return worst[a.Workflow] > worst[b.Workflow]
		}
		// Two workflows in the same state still have to be told apart, or
		// their checks stay interleaved in whatever order GitHub listed them.
		return first[a.Workflow] < first[b.Workflow]
	})
	return out
}

// hasWorkflow reports whether a check has a workflow run behind it to be
// grouped under. A StatusContext never does, and neither does a check run an
// App created: GitHub reports those with a null checkSuite.workflowRun,
// leaving RunID zero and the workflow's name empty.
func hasWorkflow(r gh.CheckRun) bool {
	return r.Kind == gh.CheckKindRun && r.RunID != 0
}

func rank(s gh.CheckState) int {
	switch s {
	case gh.CheckFailure:
		return 3
	case gh.CheckRunning, gh.CheckPending:
		return 2
	default:
		return 1
	}
}

// clamp keeps v within [0, hi].
func clamp(v, hi int) int {
	if hi < 0 {
		return 0
	}
	return min(max(v, 0), hi)
}
