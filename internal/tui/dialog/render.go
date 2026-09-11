package dialog

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/theme"
)

const (
	// starColumn holds a suggestion's star count, right-aligned.
	starColumn = 10

	// borderCols and padCols are what theme.Popup costs around its contents:
	// one column of border and one of padding on each side.
	borderCols = 2
	padCols    = 2

	// promptCols is the "> " textinput draws in front of what is typed.
	promptCols = 2
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
	for i, c := range m.candidates {
		line := padTo(c.Name, nameWidth) + rightTo(theme.Dim().Render(stars(c.Stars)), starColumn)
		if i == m.cursor {
			line = theme.Selected().Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
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
// the full width of a wide terminal.
func (m Model) boxWidth() int {
	if m.width <= 0 {
		return 40
	}
	return max(min(m.width-4, 60), 20)
}

// contentWidth is what a line inside the box may occupy. A line longer than
// this makes lipgloss wrap it, which splits a suggestion across two rows.
func (m Model) contentWidth() int {
	return max(m.boxWidth()-borderCols-padCols, 1)
}

// padTo and rightTo count display columns, not bytes: a name in Japanese
// takes two columns per character.
func padTo(s string, w int) string {
	s = ansi.Truncate(s, max(w-1, 0), "…")
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

func rightTo(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}
