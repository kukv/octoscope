package app

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// tabRowHeight is what View puts above the active tab: the tab row and the
// blank line under it. Mouse coordinates arrive relative to the whole screen
// and are shifted by this before being handed on, so a child never has to
// know it has a parent.
const tabRowHeight = 2

// tabGap separates the labels of the tab row.
const tabGap = "  "

// handleMouse routes a mouse message the way handleKey routes a key: to one
// place, not to every child. Broadcasting would let a click on the board move
// the repository list's cursor as well.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.errText != "" {
		return m, nil
	}
	// A view on top of the stack is drawn from the top of the screen, so its
	// coordinates are the screen's.
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

	if click, ok := msg.(tea.MouseClickMsg); ok && click.Y == 0 {
		if t, ok := m.tabAt(click.X); ok {
			m.tab = t
		}
		return m, nil
	}

	shifted := shiftUp(msg, tabRowHeight)
	if shifted == nil {
		return m, nil
	}
	var cmd tea.Cmd
	if m.tab == tabWork {
		m.work, cmd = m.work.Update(shifted)
	} else {
		m.repo, cmd = m.repo.Update(shifted)
	}
	return m, cmd
}

// shiftUp rebuilds msg with its row moved into the active tab's own
// coordinates. A release or a drag is dropped: nothing below acts on one, and
// forwarding them would only give each child a case to ignore.
func shiftUp(msg tea.MouseMsg, dy int) tea.Msg {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		msg.Y -= dy
		if msg.Y < 0 {
			return nil
		}
		return msg
	case tea.MouseWheelMsg:
		msg.Y -= dy
		if msg.Y < 0 {
			return nil
		}
		return msg
	}
	return nil
}

// tabAt maps a display column in the tab row onto a tab. It walks the labels
// tabRow draws, so a rename cannot move one without moving the other.
func (m Model) tabAt(x int) (tabID, bool) {
	at := 0
	for i, label := range m.tabLabels() {
		end := at + ansi.StringWidth(label)
		if x >= at && x < end {
			return tabID(i), true
		}
		at = end + len(tabGap)
	}
	return 0, false
}
