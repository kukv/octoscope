package work

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
)

// The mouse handling below reads the same geometry View draws with —
// boardTop, columnWidth, columnGap, cardRowsTop, cardLineCount — because a
// hit-test that computes the layout a second time is a hit-test that drifts.
//
// The coordinates arriving here are already relative to the board: the root
// model subtracts the height of its own tab row before forwarding.

func (m Model) handleMouseClick(msg tea.MouseClickMsg) (Model, tea.Cmd) {
	if msg.Button != tea.MouseLeft {
		return m, nil
	}
	col, row, ok := m.cardAt(msg.X, msg.Y)
	if !ok {
		return m, nil
	}
	// Clicking the selected card opens it. Bubble Tea reports no double
	// clicks, and measuring the gap between two clicks would put a clock in
	// Update; select-then-open needs neither.
	if col == m.col && row == m.row {
		if ref, ok := m.SelectedRef(); ok {
			return m, func() tea.Msg { return OpenDetailMsg{ref} }
		}
		return m, nil
	}
	m.col, m.row = col, row
	return m, nil
}

func (m Model) handleMouseWheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	// The wheel moves the cursor in the column it is over, so scrolling a
	// column the cursor is not in brings the cursor to it first.
	if col, ok := m.columnAt(msg.X); ok && col != m.col {
		m.col = col
		m.row = 0
		m.clampCursor()
	}
	switch msg.Button {
	case tea.MouseWheelUp:
		if m.row > 0 {
			m.row--
		}
	case tea.MouseWheelDown:
		if m.row+1 < len(m.work[m.section()]) {
			m.row++
		}
	}
	return m, nil
}

// columnAt maps a display column onto a board column, as an index into
// WorkSections. ok is false in the gap between two columns and past the last
// one drawn.
//
// A narrow board draws one page of columns rather than all four, so the
// position on screen is not the column's own number: the page it belongs to
// has to be added back.
func (m Model) columnAt(x int) (int, bool) {
	sections := m.visibleSections()
	w := m.columnWidth(len(sections))
	if w <= 0 || x < 0 {
		return 0, false
	}
	i := x / (w + columnGap)
	if i >= len(sections) || x%(w+columnGap) >= w {
		return 0, false
	}
	page := m.col / len(sections)
	return page*len(sections) + i, true
}

// cardAt maps a point onto the card under it. ok is false over a heading, the
// drawer, the footer, or past the last card a column has drawn.
//
// The scrolled column starts at its own offset, so the card under the pointer
// is not the nth card of the column but the nth card of what is on screen.
func (m Model) cardAt(x, y int) (col, row int, ok bool) {
	col, ok = m.columnAt(x)
	if !ok {
		return 0, 0, false
	}
	y -= m.boardTop() + headingHeight
	height := m.boardHeight()
	if y < 0 || y >= height-headingHeight {
		return 0, 0, false
	}
	section := domain.WorkSections()[col]
	row = m.cardWindow(section, height) + y/m.cardHeight()
	if row >= len(m.work[section]) {
		return 0, 0, false
	}
	return col, row, true
}
