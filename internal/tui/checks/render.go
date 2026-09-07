package checks

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

const (
	// headerHeight is the title line, the summary line and the heading rule
	// under them.
	headerHeight = 3

	// keyBarHeight is the single line at the bottom of the screen.
	keyBarHeight = 1
)

func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}
	if m.loading {
		return clip(m.spin.View()+" "+i18n.T("checks.loading"), m.width)
	}

	lines := append(m.header(), m.visibleRows()...)
	if m.declined != "" {
		lines = append(lines, m.declinedLine())
	}
	return strings.Join(append(lines, m.keyBar()), "\n")
}

func (m Model) keyBar() string {
	return theme.Dim().Render(layout.FitKeyBar(checksHints(), m.width))
}

// checksHints is the checks view's key bar, most important hint first. esc
// is first because layout.FitKeyBar never drops it: it is the only way out
// of the view.
func checksHints() []string {
	return []string{
		i18n.T("footer.checks.esc"),
		i18n.T("footer.checks.move"),
		i18n.T("footer.checks.refresh"),
	}
}

// header draws the pull request's own name and number, and beneath it the
// summary of what is wrong. The run number is left out here: with several
// workflows on screen there is no single run it could name (it is drawn on
// each workflow's own heading instead).
func (m Model) header() []string {
	title := fmt.Sprintf("%s #%d", m.ref.Repo, m.ref.Number)
	return []string{
		clip(theme.Title().Render(title), m.width),
		clip(m.summaryLine(), m.width),
		m.headingLine(),
	}
}

func (m Model) summaryLine() string {
	return theme.Dim().Render(i18n.Tf("checks.summary", map[string]any{
		"Failing": m.checks.Failed,
		"Running": m.checks.Running,
	}))
}

func (m Model) headingLine() string {
	left := theme.Heading().Render(i18n.T("checks.title"))
	rest := max(m.width-ansi.StringWidth(left), 0)
	return left + theme.Rule().Render(strings.Repeat("─", rest))
}

func (m Model) declinedLine() string {
	return clip(theme.Dim().Render(m.declined), m.width)
}

func (m Model) declinedHeight() int {
	if m.declined != "" {
		return 1
	}
	return 0
}

// paneHeight is what is left for the list once the header, the key bar and
// the declined line have been paid for.
func (m Model) paneHeight() int {
	if m.height <= 0 {
		return 0
	}
	return max(m.height-headerHeight-keyBarHeight-m.declinedHeight(), 1)
}

// visibleRows flattens the ordered checks into drawn lines, from top for
// paneHeight of them.
func (m Model) visibleRows() []string {
	lines := m.allRows()
	end := min(m.top+m.paneHeight(), len(lines))
	start := min(m.top, len(lines))
	return lines[start:end]
}

// allRows builds every line the list would draw, ungated by scrolling: the
// workflow headings and one line per check.
func (m Model) allRows() []string {
	if len(m.order) == 0 {
		return []string{clip(theme.Dim().Render(i18n.T("checks.none")), m.width)}
	}
	var lines []string
	last := ""
	first := true
	cursor := 0
	for _, r := range m.order {
		if r.Kind == gh.CheckKindRun && (first || r.Workflow != last) {
			lines = append(lines, clip(theme.Heading().Render(m.workflowTitle(r)), m.width))
			last = r.Workflow
		}
		first = false
		lines = append(lines, m.checkLine(r, cursor == m.row))
		cursor++
	}
	return lines
}

func (m Model) workflowTitle(r gh.CheckRun) string {
	if r.RunNumber > 0 {
		return fmt.Sprintf("%s #%d", r.Workflow, r.RunNumber)
	}
	return r.Workflow
}

// checkLine draws one check: its glyph, its name and, once it has one, how
// long it took.
func (m Model) checkLine(r gh.CheckRun, selected bool) string {
	text := "  " + icon.Check(r.State) + " " + r.Name
	if d := r.Duration(); d > 0 {
		text += "  " + minSec(d)
	}
	if selected {
		return theme.Selected().Render(fit(text, m.width))
	}
	return theme.Check(r.State).Render(clip(text, m.width))
}

// minSec formats a duration as m:ss, the same shape the design's mockup
// draws checks with ("2m14s" there is display-only; the on-screen form
// drops the letters).
func minSec(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// clip cuts s to w display columns. Japanese takes two columns per
// character, so the count is never a byte or a rune count.
func clip(s string, w int) string { return ansi.Truncate(s, w, "…") }

// fit clips s and pads it out to exactly w display columns.
func fit(s string, w int) string {
	s = clip(s, w)
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
