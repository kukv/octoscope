package search

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
	"github.com/kukv/octoscope/internal/usecase"
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

	lines := make([]string, len(m.saved))
	for i, q := range m.saved {
		lines[i] = m.savedRow(q, i, content)
	}
	return theme.Popup().Width(box).Render(strings.Join(lines, "\n"))
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
