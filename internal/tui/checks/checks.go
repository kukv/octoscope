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

// logMsg and logErrMsg carry a job's log answer. Both name the job they
// answer for: the cursor can move to another check while a fetch is still in
// flight, and an answer for the check the user left must not land under the
// one they moved to (d44b7fe, e1f8a99).
type logMsg struct {
	ref   gh.ItemRef
	jobID int64
	lines []gh.LogLine
}

type logErrMsg struct {
	ref   gh.ItemRef
	jobID int64
	err   error
}

// pane is which side of the view the cursor acts on.
type pane uint8

const (
	paneList pane = iota
	paneLog
)

// logPhase is where the log pane's own fetch is, independent of the
// checks list's own loading flag: the list can keep moving while a log is
// still on its way.
type logPhase uint8

const (
	phaseIdle logPhase = iota
	phaseLoading
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

	// log is the lines of the currently open job, failed steps only unless
	// failedOnly was toggled off. logJob is the id of the check that log
	// belongs to (or is being fetched for): an answer for any other job is
	// dropped rather than drawn.
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
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		return m.tick(msg)
	}
	return m, nil
}

func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	return m
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

// logArrived lands a log fetch's answer, dropping it if the user has since
// left this pull request or moved the cursor to another check.
func (m Model) logArrived(msg logMsg) Model {
	if msg.ref != m.ref || msg.jobID != m.selectedJobID() {
		return m
	}
	m.log = msg.lines
	m.logPhase = phaseIdle
	m.logRow = 0
	m.hscroll = 0
	m.errText = ""
	return m
}

func (m Model) logFailed(msg logErrMsg) Model {
	if msg.ref != m.ref || msg.jobID != m.selectedJobID() {
		return m
	}
	m.logPhase = phaseIdle
	m.errText = msg.err.Error()
	return m
}

func (m Model) tick(msg spinner.TickMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "r":
		m.loading = true
		m.declined = ""
		m.errText = ""
		return m, tea.Batch(m.spin.Tick, m.fetch())
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
		return m, m.openSelected()
	}
	return m, nil
}

func (m Model) moveRow(delta int) Model {
	if m.pane == paneLog {
		m.logRow = clamp(m.logRow+delta, len(m.logRows())-1)
		return m
	}
	m.row = clamp(m.row+delta, len(m.order)-1)
	m.declined = ""
	return m.follow()
}

// moveHscroll moves the log pane's horizontal offset, or, at either edge,
// switches which pane the cursor acts on -- the same h/l-at-the-edge
// convention the diff view's sidebar uses.
func (m Model) moveHscroll(delta int) Model {
	if m.pane != paneLog {
		if delta > 0 {
			m.pane = paneLog
		}
		return m
	}
	if delta < 0 && m.hscroll == 0 {
		m.pane = paneList
		return m
	}
	m.hscroll = max(m.hscroll+delta, 0)
	return m
}

// selectedJobID is the job id of the check under the cursor, 0 if the
// cursor is on nothing (the list is empty or has not arrived yet).
func (m Model) selectedJobID() int64 {
	if m.row < 0 || m.row >= len(m.order) {
		return 0
	}
	return m.order[m.row].JobID
}

// clearLog drops whatever log was open. It runs whenever the checks list is
// rebuilt: the cursor moves back to row 0, and any log fetch in flight was
// asked for a job that may no longer be the one under it.
func (m Model) clearLog() Model {
	m.log = nil
	m.logJob = 0
	m.logRow = 0
	m.hscroll = 0
	m.logPhase = phaseIdle
	m.errText = ""
	return m
}

// startLog asks for the selected check's log, failedOnly deciding which
// steps. It never moves the cursor to the log pane: enter can be pressed
// while still browsing the list, and L re-asks for the log already open
// without leaving the list either.
func (m Model) startLog(failedOnly bool) (Model, tea.Cmd) {
	if m.loading || len(m.order) == 0 {
		m.declined = i18n.T("checks.decline_loading")
		return m, nil
	}
	r := m.order[m.row]
	if r.Kind == gh.CheckKindStatus {
		m.declined = i18n.T("checks.decline_status_context")
		return m, nil
	}
	m.declined = ""
	m.errText = ""
	m.failedOnly = failedOnly
	m.logPhase = phaseLoading
	m.logJob = r.JobID
	m.logRow = 0
	m.hscroll = 0
	return m, m.fetchLog(r.JobID, failedOnly)
}

func (m Model) fetchLog(jobID int64, failedOnly bool) tea.Cmd {
	src, ref := m.src, m.ref
	return func() tea.Msg {
		lines, err := src.JobLog(context.Background(), ref.Repo, jobID, failedOnly)
		if err != nil {
			return logErrMsg{ref: ref, jobID: jobID, err: err}
		}
		return logMsg{ref: ref, jobID: jobID, lines: lines}
	}
}

// openSelected opens the selected check's own page: detailsUrl for a check
// run, targetUrl for a StatusContext. A failure here escalates the same way
// a failed checks fetch does -- there is nothing sensible left on screen
// once the browser could not be opened.
func (m Model) openSelected() tea.Cmd {
	if len(m.order) == 0 {
		return nil
	}
	url := m.order[m.row].URL
	if url == "" {
		return nil
	}
	src, ref := m.src, m.ref
	return func() tea.Msg {
		if err := src.OpenWeb(url); err != nil {
			return errMsg{ref: ref, err: err}
		}
		return nil
	}
}

// follow scrolls the window so the cursor stays on it.
func (m Model) follow() Model {
	h := m.paneHeight()
	if m.row < m.top {
		m.top = m.row
	}
	if m.row >= m.top+h {
		m.top = m.row - h + 1
	}
	m.top = max(m.top, 0)
	return m
}

// arrange puts the failing workflows at the top and keeps each workflow's
// checks together. A StatusContext belongs to no workflow and sorts last: it
// is the one the view can do the least with.
func arrange(runs []gh.CheckRun) []gh.CheckRun {
	worst := map[string]int{}
	first := map[string]int{}
	for i, r := range runs {
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
		if a.Workflow == b.Workflow {
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
