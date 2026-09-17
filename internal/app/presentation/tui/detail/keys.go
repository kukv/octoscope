// Key handling, one function per mode. Which keys do anything depends on what
// is on the screen, and keeping the modes apart is what stops a key meant for
// the composer from reaching the body underneath it (.claude/rules/tui.md).

package detail

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/merge"
)

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// An overlay's keys have nothing to act on until its fetch answers.
	// modeView is the exception: the body is drawn, and q still leaves.
	if m.mode != modeView && m.phase == phaseLoading {
		return m, nil
	}
	switch m.mode {
	case modeCompose:
		return m.handleComposeKey(msg)
	case modeConfirm:
		return m.handleConfirmKey(msg)
	case modePick:
		return m.handlePickerKey(msg)
	case modeSubmit:
		return m.handleSubmitKey(msg)
	case modeMerge:
		return m.handleMergeKey(msg)
	case modeView:
	}
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "o":
		if m.url == "" {
			return m, nil
		}
		return m, m.openWeb(m.ref, m.url)
	case "d":
		// An issue has no diff.
		if m.ref.Kind != domain.ItemPR {
			return m, nil
		}
		ref := m.ref
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
	case "s":
		// An issue has no checks.
		if m.ref.Kind != domain.ItemPR {
			return m, nil
		}
		ref := m.ref
		return m, func() tea.Msg { return OpenChecksMsg{Ref: ref} }
	case "m":
		// An issue has nothing to merge.
		if m.ref.Kind != domain.ItemPR {
			return m, nil
		}
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		// Nor has a pull request that is already merged or closed. GitHub
		// answers UNKNOWN for a merged one, so the popup would say it is
		// still working the answer out, for ever.
		if !m.canMerge() {
			return m, nil
		}
		m.mode, m.phase = modeMerge, phaseIdle
		m.errText = ""
		m.merge = merge.New(m.src, m.ref)
		m.merge, _ = m.merge.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.merge.Init()
	case "r":
		m.phase = phaseLoading
		m.declined = ""
		return m, fetch(m.src, m.ref)
	case "c":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		m.mode, m.phase = modeCompose, phaseIdle
		m.errText = ""
		m.textarea.Reset()
		m.textarea.Focus()
		return m, textarea.Blink
	case "x":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		if _, ok := m.stateAction(); !ok {
			return m, nil // merged and the like: no action
		}
		m.mode, m.phase = modeConfirm, phaseIdle
		m.errText = ""
		return m, nil
	case "v":
		// An issue has no review. Unlike the diff view's v, this always
		// fetches first: detail holds no review context of its own.
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		if m.ref.Kind != domain.ItemPR {
			return m, nil
		}
		m.mode, m.phase = modeSubmit, phaseLoading
		m.errText = ""
		return m, fetchReviewContext(m.src, m.ref)
	case "l":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		m.mode, m.phase = modePick, phaseLoading
		m.errText = ""
		return m, fetchLabelPicker(m.src, m.ref)
	case "a":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		m.mode, m.phase = modePick, phaseLoading
		m.errText = ""
		return m, fetchAssigneePicker(m.src, m.ref)
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

func (m Model) handlePickerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.phase == phaseWorking {
		return m, nil // ignore every other key while the edit is in flight
	}
	switch msg.String() {
	case "esc":
		m.mode, m.phase = modeView, phaseIdle
		m.errText = ""
		return m, nil
	case "j", "down":
		m.picker.moveDown(visibleRows(m.height))
		return m, nil
	case "k", "up":
		m.picker.moveUp()
		return m, nil
	case " ", "space":
		m.picker.toggle()
		return m, nil
	case "enter":
		add, remove := m.picker.diff()
		if len(add) == 0 && len(remove) == 0 {
			m.mode, m.phase = modeView, phaseIdle // nothing changed: just close
			m.errText = ""
			return m, nil
		}
		m.phase = phaseWorking
		m.errText = ""
		return m, applyPicker(m.src, m.ref, m.picker.kind, add, remove)
	}
	return m, nil
}

func (m Model) handleSubmitKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.submit, cmd = m.submit.Update(msg)
	return m, cmd
}

func (m Model) handleMergeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.merge, cmd = m.merge.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.phase == phaseWorking {
		return m, nil // ignore every other key while the change is in flight
	}
	switch msg.String() {
	case "y":
		closing, ok := m.stateAction()
		if !ok {
			m.mode, m.phase = modeView, phaseIdle
			return m, nil
		}
		m.phase = phaseWorking
		m.errText = ""
		return m, setState(m.src, m.ref, closing)
	case "n", "esc":
		m.mode, m.phase = modeView, phaseIdle
		m.errText = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleComposeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.phase == phaseWorking {
		return m, nil // ignore every other key while the comment is in flight
	}
	switch msg.String() {
	case "esc":
		m.mode, m.phase = modeView, phaseIdle
		m.errText = ""
		m.textarea.Reset()
		return m, nil
	case "ctrl+s":
		if strings.TrimSpace(m.textarea.Value()) == "" {
			return m, nil // an empty body is not sent
		}
		m.phase = phaseWorking
		m.errText = ""
		return m, postComment(m.src, m.ref, m.textarea.Value())
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}
