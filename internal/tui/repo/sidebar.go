package repo

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// The sidebar's geometry, read by both View and the hit-test: a fixed column
// of names with their badges, then the rule between the panes.
const (
	sidebarWidth = 30
	sidebarRule  = 1

	// sidebarTop is the first line a repository is drawn on: the heading and
	// the blank line under it. The hit-test reads the same constant.
	sidebarTop = 2

	// badgeColumn holds "12/3" right-aligned, or the dash a repository that
	// could not be counted gets.
	badgeColumn = 8

	// minSidebarWidth is where the sidebar folds away (spec §4.6, design §9).
	minSidebarWidth = 100

	// addButtonHeight is the blank line and the add button under the
	// repositories. The rows have to give up that much, or the sidebar grows
	// past the bottom of the terminal.
	addButtonHeight = 2
)

// sidebarCols is how much of the terminal the sidebar takes, rule included.
// Zero means it is folded away.
func (m Model) sidebarCols() int {
	if m.width < minSidebarWidth || len(m.rows) == 0 {
		return 0
	}
	return sidebarWidth + sidebarRule
}

// bodyWidth is what is left for the table once the sidebar has taken its
// share. Every column in the table reads this instead of m.width, or a
// repository name long enough to open the sidebar pushes rows off the right
// edge of the terminal.
func (m Model) bodyWidth() int {
	return max(m.width-m.sidebarCols(), 1)
}

// selectedRepo is the repository under the sidebar's cursor: what fetchList
// asks for and what the ref in SelectedRef names. It is empty before the
// settings file's list and the working directory's own repository are both
// known, which fetchList passes straight to ListPRs/ListIssues: an empty
// repository there falls back to the client's own. header substitutes its
// own placeholder for that case; this one must not.
func (m Model) selectedRepo() string {
	if len(m.rows) == 0 {
		return ""
	}
	return m.rows[m.selected].name
}

// sidebarRows is how many repositories fit under the heading, and
// sidebarWindow is the first one drawn, chosen to keep the cursor in view.
// The list is realistically 20-50 long and the terminal is not (design §2),
// so it scrolls exactly as the table does.
func (m Model) sidebarRows() int {
	if m.height <= 0 {
		return len(m.rows)
	}
	return max(m.height-sidebarTop-addButtonHeight-footerHeight, 1)
}

func (m Model) sidebarWindow() int {
	if m.selected < m.sidebarRows() {
		return 0
	}
	return m.selected - m.sidebarRows() + 1
}

// sidebar draws one line per repository on screen: its name, and how much is
// open in it. A repository whose count could not be fetched keeps its row and
// loses its numbers.
func (m Model) sidebar() []string {
	lines := []string{theme.Dim().Render(pad(i18n.T("repos.sidebar"), sidebarWidth)), ""}
	first := m.sidebarWindow()
	for i := first; i < min(first+m.sidebarRows(), len(m.rows)); i++ {
		r := m.rows[i]
		badge := "—"
		if r.counted {
			badge = fmt.Sprintf("%d/%d", r.prs, r.issues)
		}
		line := pad(r.name, sidebarWidth-badgeColumn) +
			right(theme.Dim().Render(badge), badgeColumn)
		switch {
		case i == m.selected && m.focus == paneSidebar:
			line = theme.Selected().Render(line)
		case i == m.selected:
			line = theme.Accent().Render(line)
		}
		lines = append(lines, line)
	}
	return append(lines, "", theme.Dim().Render(pad(i18n.T("repos.add_button"), sidebarWidth)))
}

// addRowY is the line the add button is drawn on: under the repositories on
// screen, with one blank line between. The hit-test reads this rather than
// counting the drawn lines a second time.
func (m Model) addRowY() int {
	return sidebarTop + min(m.sidebarRows(), len(m.rows)) + 1
}

// joinPanes puts the sidebar beside the body, padding whichever is shorter so
// the rule runs the full height of the taller one.
func joinPanes(left, right []string, leftWidth int) []string {
	n := max(len(left), len(right))
	out := make([]string, n)
	for i := range n {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = padExact(l, leftWidth) + theme.Rule().Render("│") + r
	}
	return out
}

// padExact pads s out to exactly w columns with no ellipsis. sidebar()
// already fits every line to leftWidth; running it through pad() here would
// reserve pad's usual one-column margin and clip a name that already fits.
func padExact(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
