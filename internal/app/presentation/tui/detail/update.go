// Update and the handlers it dispatches to. Each message type has a handler
// of its own rather than a branch inside Update, so that what happens when a
// fetch answers can be read without reading what happens when a key is
// pressed.

package detail

import (
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/merge"
	"github.com/kukv/octoscope/internal/app/presentation/tui/review"
	"github.com/kukv/octoscope/internal/i18n"
)

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg), nil
	case spinner.TickMsg:
		return m.tick(msg)
	case itemMsg:
		return m.itemArrived(msg), nil
	case commentPostedMsg:
		return m.commentPosted(msg)
	case commentErrorMsg:
		return m.commentFailed(msg), nil
	case stateChangedMsg:
		return m.stateChanged(msg)
	case stateErrorMsg:
		return m.stateFailed(msg), nil
	case pickerCandidatesMsg:
		return m.candidatesArrived(msg), nil
	case pickerAppliedMsg:
		return m.pickApplied(msg)
	case pickErrorMsg:
		return m.pickFailed(msg), nil
	case reviewContextMsg:
		return m.reviewContextArrived(msg), nil
	case reviewContextErrMsg:
		return m.reviewContextFailed(msg), nil
	case review.CancelledMsg:
		return m.submitCancelled(), nil
	case review.SubmittedMsg:
		return m.submitDone()
	case review.ErrorMsg:
		return m.submitFailed(msg)
	case merge.CancelledMsg:
		return m.mergeCancelled(), nil
	case merge.MergedMsg:
		return m.mergeDone(msg)
	case merge.ErrorMsg:
		return m.mergeFailed(msg)
	case errMsg:
		return m.fetchFailed(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	// The merge popup fetches for itself, and its answer is a type this view
	// cannot name. Everything left over while it is open is its: sizes, keys
	// and its three public messages have already returned above.
	if m.mode == modeMerge {
		var cmd tea.Cmd
		m.merge, cmd = m.merge.Update(msg)
		return m, cmd
	}
	if msg, ok := msg.(tea.MouseWheelMsg); ok {
		return m.wheel(msg)
	}
	return m, nil
}

func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	m.relayout()
	m.textarea.SetWidth(msg.Width)
	m.textarea.SetHeight(max(msg.Height-6, 3))
	if m.submit.Active() {
		m.submit, _ = m.submit.Update(msg)
	}
	if m.merge.Active() {
		m.merge, _ = m.merge.Update(msg)
	}
	return m
}

func (m Model) tick(msg spinner.TickMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) itemArrived(msg itemMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	it := msg.item
	m.phase = phaseIdle
	m.state = it.State
	m.errText = ""
	m.declined = ""
	m.labels = labelNames(it.Labels)
	m.assignees = authorLogins(it.Assignees)
	m.url = it.URL
	m.item, m.loaded = it, true
	if it.Kind == domain.ItemPR {
		m.title = i18n.Tf("detail.pr_title", map[string]any{"Number": it.Number, "Title": it.Title})
	} else {
		m.title = i18n.Tf("detail.issue_title", map[string]any{"Number": it.Number, "Title": it.Title})
	}
	m.relayout()
	m.body.GotoTop()
	return m
}

func (m Model) commentPosted(msg commentPostedMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.errText = ""
	m.textarea.Reset()
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) commentFailed(msg commentErrorMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	m.phase = phaseIdle
	m.errText = msg.err.Error()
	return m
}

func (m Model) stateChanged(msg stateChangedMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.errText = ""
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) stateFailed(msg stateErrorMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = msg.err.Error()
	return m
}

func (m Model) candidatesArrived(msg pickerCandidatesMsg) Model {
	if msg.ref != m.ref {
		return m // an answer for an item the user has already left
	}
	if msg.kind == pickLabels {
		names := make([]string, len(msg.labels))
		colors := make(map[string]string, len(msg.labels))
		for i, l := range msg.labels {
			names[i] = l.Name
			colors[l.Name] = l.Color
		}
		m.picker = newPicker(pickLabels, i18n.T("picker.labels"), names, colors, m.labels)
	} else {
		m.picker = newPicker(pickAssignees, i18n.T("picker.assignees"), msg.users, nil, m.assignees)
	}
	m.mode, m.phase = modePick, phaseIdle
	return m
}

func (m Model) pickApplied(msg pickerAppliedMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) pickFailed(msg pickErrorMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	if m.phase == phaseWorking { // the apply failed; the picker stays up
		m.phase = phaseIdle
	} else { // the candidates never arrived; there is no picker to show
		m.mode, m.phase = modeView, phaseIdle
	}
	m.errText = msg.err.Error()
	return m
}

func (m Model) reviewContextArrived(msg reviewContextMsg) Model {
	if msg.ref != m.ref {
		return m // an answer for an item the user has already left
	}
	target := review.Target{
		PullRequest:     msg.ctx.PullRequest,
		Pending:         msg.ctx.Pending,
		PendingComments: msg.ctx.PendingCount(),
	}
	m.submit = review.New(m.src, target)
	m.submit, _ = m.submit.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.mode, m.phase = modeSubmit, phaseIdle
	m.errText = ""
	return m
}

func (m Model) reviewContextFailed(msg reviewContextErrMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = msg.err.Error()
	return m
}

func (m Model) submitCancelled() Model {
	if m.mode != modeSubmit {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = ""
	return m
}

func (m Model) submitDone() (Model, tea.Cmd) {
	if m.mode != modeSubmit {
		return m, nil
	}
	m.errText = ""
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) submitFailed(msg review.ErrorMsg) (Model, tea.Cmd) {
	if m.mode != modeSubmit {
		return m, nil
	}
	var cmd tea.Cmd
	m.submit, cmd = m.submit.Update(msg)
	m.errText = msg.Err.Error()
	return m, cmd
}

func (m Model) mergeCancelled() Model {
	if m.mode != modeMerge {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = ""
	// The zero popup is the inactive one: without this Active() would keep
	// handing it window sizes after it is gone.
	m.merge = merge.Model{}
	return m
}

// mergeDone leaves the view when the pull request was merged: a merged pull
// request is not something to keep reading. Joining or leaving the auto-merge
// queue leaves it open, so the popup keeps the screen and refetches instead.
// The board and the Repos list are refreshed by the root either way, which
// sees the same merge message this one came from, so nothing is sent on.
func (m Model) mergeDone(msg merge.MergedMsg) (Model, tea.Cmd) {
	if m.mode != modeMerge {
		return m, nil
	}
	if !msg.Merged {
		var cmd tea.Cmd
		m.merge, cmd = m.merge.Update(msg)
		return m, cmd
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = ""
	m.merge = merge.Model{}
	return m, func() tea.Msg { return ClosedMsg{} }
}

// mergeFailed shows the failure the popup on screen raised. A popup that was
// closed and opened again leaves its own request in flight, and that one's
// failure must not land in this view's footer.
func (m Model) mergeFailed(msg merge.ErrorMsg) (Model, tea.Cmd) {
	if m.mode != modeMerge || !m.merge.Owns(msg) {
		return m, nil
	}
	var cmd tea.Cmd
	m.merge, cmd = m.merge.Update(msg)
	m.errText = msg.Err.Error()
	return m, cmd
}

func (m Model) fetchFailed(msg errMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.phase = phaseIdle
	err := msg.err
	return m, func() tea.Msg { return ErrorMsg{err} }
}

func (m Model) wheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	// The body is drawn only with nothing over it and nothing in flight.
	if m.mode != modeView || m.phase != phaseIdle {
		return m, nil
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

// stillLoading is what the keys that need the item answer with while it is
// on its way. Against the real API the item was still not there four seconds
// after it was asked for, which is long enough for a key that does nothing
// to read as a key that is broken.
func (m Model) stillLoading() Model {
	m.declined = i18n.T("detail.decline_loading")
	return m
}
