package dialog

import (
	"fmt"
	"strings"

	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

const (
	// starColumn holds a suggestion's star count, right-aligned.
	starColumn = 10

	// promptCols is the "> " textinput draws in front of what is typed.
	promptCols = 2

	// chromeRows is every line of the box that is not a suggestion: the
	// border's two, the title and the blank under it, the field, the hint,
	// the blank above the suggestions and the empty line they end on.
	chromeRows = 8
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(theme.Title().Render(m.title) + "\n\n")
	b.WriteString(m.input.View() + "\n")
	b.WriteString(theme.Dim().Render(m.hint) + "\n")
	b.WriteString(m.candidateLines())
	if m.errText != "" {
		b.WriteString("\n" + theme.Error().Render(m.errText))
	}
	// lipgloss counts Width from the outside in: the border and the padding
	// come out of it, not on top of it, which is why contentWidth subtracts
	// both.
	return theme.Popup().Width(m.boxWidth()).Render(b.String())
}

// candidateLines is the suggestion list under the field: what is running,
// what came back, or that nothing matched.
func (m Model) candidateLines() string {
	if m.searching {
		return "\n" + theme.Dim().Render(i18n.T("dialog.searching")) + "\n"
	}
	if len(m.candidates) == 0 {
		return "\n" + theme.Dim().Render(i18n.T("dialog.no_candidates")) + "\n"
	}
	var b strings.Builder
	b.WriteString("\n")
	nameWidth := max(m.contentWidth()-starColumn, 1)
	rows := m.candidateRows()
	first := m.candidateWindow(rows)
	for i := first; i < min(first+rows, len(m.candidates)); i++ {
		c := m.candidates[i]
		line := layout.Pad(c.Name, nameWidth) + layout.Right(theme.Dim().Render(stars(c.Stars)), starColumn)
		if i == m.cursor {
			line = theme.SelectedLine(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

// candidateRows is how many suggestions fit the height the holder gave,
// once the rest of the box and (while one is up) the error have taken their
// share. With no height yet it draws them all.
func (m Model) candidateRows() int {
	if m.height <= 0 {
		return len(m.candidates)
	}
	rows := m.height - chromeRows
	if m.errText != "" {
		rows--
	}
	return max(rows, 1)
}

// candidateWindow is the first suggestion drawn, chosen to keep the cursor
// in view -- the same top-anchored scroll the Search tab's saved-query
// picker uses. The field (cursor -1) keeps the list at its top.
func (m Model) candidateWindow(rows int) int {
	if m.cursor < rows {
		return 0
	}
	return m.cursor - rows + 1
}

// stars is the count beside a suggestion. A repository with none gets
// nothing: a bare zero reads as a measurement rather than as an absence.
func stars(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("★ %d", n)
}

// boxWidth keeps the popup readable at eighty columns without letting it run
// the full width of a wide terminal. The judgment is shared with
// internal/app/presentation/tui/search's saved-queries popup, so it lives in layout; only
// the type is not shared.
func (m Model) boxWidth() int {
	return layout.PopupWidth(m.width)
}

// contentWidth is what a line inside the box may occupy. A line longer than
// this makes lipgloss wrap it, which splits a suggestion across two rows.
func (m Model) contentWidth() int {
	return layout.PopupContentWidth(m.boxWidth())
}
