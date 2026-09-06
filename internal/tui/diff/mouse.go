package diff

import tea "charm.land/bubbletea/v2"

// The mouse handling below reads the same geometry render.go draws with --
// showSidebar, sidebarWidth, m.gutter, headerHeight, paneHeight and m.top --
// because a hit-test that computes the layout a second time is a hit-test
// that drifts.

// handleMouseClick maps a click onto the pane under it. While the composer
// or a popup is open the mouse must not reach the panes underneath, the same
// rule handleKey follows.
func (m Model) handleMouseClick(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if m.composing || m.posting || m.submitting || m.discarding {
		return m, nil
	}
	if m.showSidebar() && msg.X < sidebarWidth {
		i := m.fileAt(msg.Y)
		if i < 0 {
			return m, nil
		}
		m.file = i
		m.sidebar = true
		m.top = 0
		m.rows = m.buildRows()
		m.row = firstRow(m.rows)
		m = m.follow()
		return m.followSidebar(), nil
	}
	i := m.rowAt(msg.Y)
	if i < 0 {
		return m, nil
	}
	// Clicking the selected row opens it -- if it has anything to open.
	// There is no double click to detect (spec 4.0): Bubble Tea does not
	// report one, and timing two clicks ourselves would put a clock in
	// Update.
	if i == m.row && !m.sidebar {
		return m.toggleCollapsed(), nil
	}
	m.row = i
	m.sidebar = false
	return m, nil
}

// handleMouseWheel moves whatever is under the pointer: the file list over
// the sidebar, the diff pane otherwise.
func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	if m.composing || m.posting || m.submitting || m.discarding {
		return m, nil
	}
	delta := 1
	if msg.Button == tea.MouseWheelUp {
		delta = -1
	}
	if m.showSidebar() && msg.X < sidebarWidth {
		return m.moveFile(delta), nil
	}
	// moveRow redirects to moveFile while the sidebar is selected, which is
	// right for j/k but not for a wheel over the diff pane: the pointer,
	// not where the cursor last was, decides what moves.
	m.sidebar = false
	return m.moveRow(delta), nil
}

// rowAt maps a screen position onto an index into m.rows, or -1 when the
// position is not on the diff pane. m.top is the scroll offset the view is
// drawing with, which is why the hit test reads it rather than assuming
// zero.
func (m Model) rowAt(y int) int {
	if y < headerHeight || y >= headerHeight+m.paneHeight() {
		return -1
	}
	i := m.top + (y - headerHeight)
	if i >= len(m.rows) {
		return -1
	}
	return i
}

// fileAt maps a screen position onto an index into m.files, or -1 when the
// position is not on a file (the gap under the last one, say). Each file
// takes two lines -- its path and its size -- the same pairing sidebarLines
// draws and followSidebar counts by.
func (m Model) fileAt(y int) int {
	if y < headerHeight || y >= headerHeight+m.paneHeight() {
		return -1
	}
	i := m.fileTop + (y-headerHeight)/2
	if i >= len(m.files) {
		return -1
	}
	return i
}
