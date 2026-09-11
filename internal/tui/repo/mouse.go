package repo

import (
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The list's own geometry, read by both View and the hit-test below: the
// title, a blank line, the sub-tab row, another blank line, and then one line
// per item. A hit-test that computed this a second time would drift from the
// drawing.
const (
	subTabGap = "  "
	subTabRow = 2
	listTop   = 4
)

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	if m.sidebarCols() > 0 && msg.X < sidebarWidth {
		if msg.Y == m.addRowY() {
			return m.openAddDialog()
		}
		row, ok := m.sidebarRowAt(msg.Y)
		if !ok {
			return m, nil
		}
		m.focus = paneSidebar
		// Clicking the selected repository has nothing to open: the right
		// pane already shows it.
		if row == m.selected {
			return m, nil
		}
		return m.selectRow(row)
	}
	msg.X -= m.sidebarCols()
	m.focus = paneList
	if msg.Y == subTabRow {
		return m.showTab(m.subTabAt(msg.X))
	}
	// The list scrolls, so the row under the pointer is the nth row on screen,
	// not the nth item of the list.
	row := msg.Y - listTop
	if row < 0 || row >= m.visibleRows() {
		return m, nil
	}
	row += m.rowWindow(m.visibleRows())
	if row >= m.itemCount() {
		return m, nil
	}
	// Clicking the selected row opens it. Bubble Tea reports no double clicks,
	// and measuring the gap between two would put a clock in Update.
	if row == m.cursors[m.tab] {
		if ref, ok := m.SelectedRef(); ok {
			return m, func() tea.Msg { return OpenDetailMsg{ref} }
		}
		return m, nil
	}
	m.cursors[m.tab] = row
	return m, nil
}

// sidebarRowAt maps a screen line onto a repository, the same way rows are
// scrolled into view for drawing. ok is false above the list, in the gap
// under the heading, and past the last repository.
func (m Model) sidebarRowAt(y int) (int, bool) {
	if y < sidebarTop || y >= sidebarTop+m.sidebarRows() {
		return 0, false
	}
	row := y - sidebarTop + m.sidebarWindow()
	if row >= len(m.rows) {
		return 0, false
	}
	return row, true
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	if m.sidebarCols() > 0 && msg.X < sidebarWidth {
		switch msg.Button {
		case tea.MouseWheelUp:
			if m.selected > 0 {
				return m.selectRow(m.selected - 1)
			}
		case tea.MouseWheelDown:
			if m.selected < len(m.rows)-1 {
				return m.selectRow(m.selected + 1)
			}
		}
		return m, nil
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		if m.cursors[m.tab] > 0 {
			m.cursors[m.tab]--
		}
	case tea.MouseWheelDown:
		if n := m.itemCount(); n > 0 && m.cursors[m.tab] < n-1 {
			m.cursors[m.tab]++
		}
	}
	return m, nil
}

// subTabAt maps a display column onto a sub-tab. ok is false in the gap
// between them and past the last one.
func (m Model) subTabAt(x int) (tabID, bool) {
	at := 0
	for i, label := range subTabLabels() {
		end := at + ansi.StringWidth(label)
		if x >= at && x < end {
			return tabID(i), true
		}
		at = end + len(subTabGap)
	}
	return 0, false
}

// showTab switches to t, fetching its list the first time, exactly as the tab
// key does.
func (m Model) showTab(t tabID, ok bool) (Model, tea.Cmd) {
	if !ok || t == m.tab {
		return m, nil
	}
	m.tab = t
	if !m.loaded[m.tab] && len(m.rows) > 0 {
		m.loading[m.tab] = true
		return m, fetchList(m.src, m.tab, m.selectedRepo(), m.gen)
	}
	return m, nil
}
