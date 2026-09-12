package search

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
	"github.com/kukv/octoscope/internal/usecase"
)

// pickerBorderRows and pickerFooterRows are what stands between the
// terminal's height and the rows left for saved queries: theme.Popup's own
// top and bottom border, and the key bar View draws under the popup.
const (
	pickerBorderRows = 2
	pickerFooterRows = 1
)

// pickerView draws the saved queries as a popup box, the width theme.Popup
// and layout.PopupWidth give the add-repository dialog. It holds no input
// field of its own -- it is picked from, not typed into.
func (m Model) pickerView() string {
	box := layout.PopupWidth(m.width)
	content := layout.PopupContentWidth(box)

	if len(m.saved) == 0 {
		return theme.Popup().Width(box).Render(theme.Dim().Render(i18n.T("search.no_saved_queries")))
	}

	rows := m.pickerRows()
	first := m.pickerWindow(rows)
	last := min(first+rows, len(m.saved))
	lines := make([]string, 0, last-first)
	for i := first; i < last; i++ {
		lines = append(lines, m.savedRow(m.saved[i], i, content))
	}
	return theme.Popup().Width(box).Render(strings.Join(lines, "\n"))
}

// pickerRows is how many saved-query rows fit once the border, key bar and
// (while one is up) the notice line have taken their share. Unset
// (m.height <= 0), it draws them all, the way the rest of this tab behaves
// before its first size arrives.
func (m Model) pickerRows() int {
	if m.height <= 0 {
		return len(m.saved)
	}
	rows := m.height - pickerBorderRows - pickerFooterRows
	if m.notice != "" {
		rows--
	}
	return max(rows, 1)
}

// pickerWindow is the first row drawn, chosen to keep the cursor in view --
// the same top-anchored scroll resultWindow uses for the result pane.
func (m Model) pickerWindow(rows int) int {
	if m.pick < rows {
		return 0
	}
	return m.pick - rows + 1
}

// savedRow draws one entry: its name, and its query beside it in a dimmer
// colour if there is room. layout.Clip cuts it to the box instead of
// leaving lipgloss to wrap it, which would split the row across two lines.
func (m Model) savedRow(q usecase.SavedQuery, i int, width int) string {
	line := q.Name
	if rest := width - ansi.StringWidth(line) - 1; rest > 0 {
		line += " " + theme.Dim().Render(layout.Clip(q.Query, rest))
	}
	line = layout.Clip(line, width)
	if i == m.pick {
		return theme.Selected().Render(line)
	}
	return line
}
