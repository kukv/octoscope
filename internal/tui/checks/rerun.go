package checks

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// mode is which overlay is on screen, the same shape internal/tui/diff's
// own mode/phase pair uses.
type mode uint8

const (
	modeView mode = iota
	modeRerun
)

func (m mode) String() string {
	switch m {
	case modeView:
		return "view"
	case modeRerun:
		return "rerun"
	}
	return "mode(?)"
}

// rerunPhase is where the rerun popup is in its round trip: choosing a
// scope, or waiting for RerunWorkflow to answer.
type rerunPhase uint8

const (
	rerunIdle rerunPhase = iota
	rerunWorking
)

func (p rerunPhase) String() string {
	switch p {
	case rerunIdle:
		return "idle"
	case rerunWorking:
		return "working"
	}
	return "rerunPhase(?)"
}

// rerunDoneMsg and rerunErrMsg carry RerunWorkflow's answer for the run it
// was sent for, dropped the same way logMsg/logErrMsg are if the user has
// since left this pull request or started a rerun on another check.
type rerunDoneMsg struct {
	ref   gh.ItemRef
	runID int64
}

type rerunErrMsg struct {
	ref   gh.ItemRef
	runID int64
	err   error
}

// startRerun opens the popup for the selected check's workflow, captured
// now rather than read again from m.order at send time: a refresh landing
// while the popup is open would rebuild m.order and could move what row
// m.row now points at.
func (m Model) startRerun() Model {
	if m.loading || len(m.order) == 0 {
		m.declined = i18n.T("checks.decline_loading")
		return m
	}
	r := m.order[m.row]
	// A check run an App created has a null workflowRun behind it and so no
	// run id: there is nothing for RerunWorkflow to name.
	if r.Kind == gh.CheckKindStatus || r.RunID == 0 {
		m.declined = i18n.T("checks.decline_rerun_status_context")
		return m
	}
	m.declined = ""
	m.errText = ""
	m.mode = modeRerun
	m.rerunPhase = rerunIdle
	m.rerunScope = gh.RerunFailed
	m.rerunRunID = r.RunID
	m.rerunWorkflow = m.workflowTitle(r)
	return m
}

// handleRerunKey handles every key while the popup is up. It swallows keys
// while a rerun is in flight, the same way the diff view's discard
// confirmation does, so a second enter cannot send it twice.
func (m Model) handleRerunKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.rerunPhase == rerunWorking {
		return m, nil
	}
	switch msg.String() {
	case "j", "down":
		m.rerunScope = gh.RerunAll
		return m, nil
	case "k", "up":
		m.rerunScope = gh.RerunFailed
		return m, nil
	case "enter":
		return m.sendRerun()
	case "esc":
		m.mode = modeView
		m.rerunPhase = rerunIdle
		m.errText = ""
		return m, nil
	}
	return m, nil
}

func (m Model) sendRerun() (Model, tea.Cmd) {
	m.rerunPhase = rerunWorking
	m.errText = ""
	src, ref, runID, scope := m.src, m.ref, m.rerunRunID, m.rerunScope
	return m, func() tea.Msg {
		if err := src.RerunWorkflow(context.Background(), ref.Repo, runID, scope); err != nil {
			return rerunErrMsg{ref: ref, runID: runID, err: err}
		}
		return rerunDoneMsg{ref: ref, runID: runID}
	}
}

// rerunDone closes the popup and says so at footer level. It does not
// refetch the checks list: a run just started still shows as it did before,
// and r is what the user presses once it has something new to show.
func (m Model) rerunDone(msg rerunDoneMsg) Model {
	if msg.ref != m.ref || msg.runID != m.rerunRunID {
		return m
	}
	m.mode = modeView
	m.rerunPhase = rerunIdle
	m.declined = i18n.T("checks.rerun_started")
	return m
}

func (m Model) rerunFailed(msg rerunErrMsg) Model {
	if msg.ref != m.ref || msg.runID != m.rerunRunID {
		return m
	}
	m.rerunPhase = rerunIdle
	m.errText = msg.err.Error()
	return m
}
