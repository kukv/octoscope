package checks

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

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

// logPhase is where the log pane's own fetch is, independent of the
// checks list's own loading flag: the list can keep moving while a log is
// still on its way.
type logPhase uint8

const (
	phaseIdle logPhase = iota
	phaseLoading
)

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
	m.hscroll = clamp(m.hscroll+delta, m.maxHscroll())
	return m
}

// boundHscroll pulls the log pane's offset back inside what is on screen.
// The bound is the widest line in the window, so anything that moves the
// window -- scrolling it, or resizing the terminal -- can leave an offset
// that was legal past every line there now is, and the pane draws blank.
func (m Model) boundHscroll() Model {
	m.hscroll = min(m.hscroll, m.maxHscroll())
	return m
}

// maxHscroll is how far right the log pane can be scrolled before the widest
// line on screen has gone past its left edge, leaving it blank.
func (m Model) maxHscroll() int {
	rows := m.logRows()
	end := min(m.logRow+m.paneHeight(), len(rows))
	start := min(m.logRow, len(rows))
	widest := 0
	for _, r := range rows[start:end] {
		widest = max(widest, ansi.StringWidth(r))
	}
	return max(widest-m.logWidth(), 0)
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
	if m.loading {
		m.declined = i18n.T("checks.decline_loading")
		return m, nil
	}
	if len(m.order) == 0 {
		m.declined = i18n.T("checks.none")
		return m, nil
	}
	r := m.order[m.row]
	// A check run an App created reports a check run id where a workflow's
	// check reports an Actions job id, and gh has no job to serve for it.
	if !hasWorkflow(r) {
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
