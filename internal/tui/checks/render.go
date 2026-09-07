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

	// listWidth is the check list's fixed width, matching the diff view's
	// own sidebar.
	listWidth = 22
)

func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}
	if m.loading {
		return clip(m.spin.View()+" "+i18n.T("checks.loading"), m.width)
	}

	lines := append(m.header(), m.body()...)
	if m.errText != "" {
		lines = append(lines, m.errLine())
	}
	if m.declined != "" {
		lines = append(lines, m.declinedLine())
	}
	if m.mode == modeRerun {
		lines = append(lines, m.rerunLines()...)
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
		i18n.T("footer.checks.log"),
		i18n.T("footer.checks.full"),
		i18n.T("footer.checks.rerun"),
		i18n.T("footer.checks.pane"),
		i18n.T("footer.checks.open"),
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

// headingLine draws the rule under the header, with the list's own heading
// on the left and the log pane's current mode -- failed steps or full log --
// where the divider crosses into it.
func (m Model) headingLine() string {
	left := fit(theme.Heading().Render(i18n.T("checks.title")), listWidth)
	div := theme.Rule().Render("│")
	label := theme.Heading().Render(m.logModeLabel())
	rest := max(m.width-listWidth-ansi.StringWidth(div)-ansi.StringWidth(label), 0)
	return left + div + label + theme.Rule().Render(strings.Repeat("─", rest))
}

func (m Model) logModeLabel() string {
	if m.failedOnly {
		return i18n.T("checks.log_failed_only")
	}
	return i18n.T("checks.log_full")
}

func (m Model) errLine() string {
	text := theme.Error().Render(i18n.T("common.error_prefix")) + singleLine(m.errText)
	return clip(text, m.width)
}

func (m Model) errHeight() int {
	if m.errText != "" {
		return 1
	}
	return 0
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

// rerunLines draws the rerun popup: a blank separator, the title naming the
// workflow, the two scopes with the selected one picked out, and either a
// failed send's error or the spinner while it is in flight.
func (m Model) rerunLines() []string {
	title := i18n.Tf("checks.rerun_title", map[string]any{"Workflow": m.rerunWorkflow})
	lines := []string{
		"",
		clip(theme.Title().Render(title), m.width),
		clip(m.rerunOption(gh.RerunFailed, i18n.T("checks.rerun_failed_only")), m.width),
		clip(m.rerunOption(gh.RerunAll, i18n.T("checks.rerun_all")), m.width),
	}
	switch {
	case m.errText != "":
		lines = append(lines, clip(theme.Error().Render(i18n.T("common.error_prefix"))+singleLine(m.errText), m.width))
	case m.rerunPhase == rerunWorking:
		lines = append(lines, clip(m.spin.View()+" "+i18n.T("confirm.working"), m.width))
	}
	return lines
}

func (m Model) rerunOption(scope gh.RerunScope, text string) string {
	if scope == m.rerunScope {
		return theme.Selected().Render(text)
	}
	return text
}

// rerunHeight is what rerunLines takes, out of the pane's height budget the
// same way declinedHeight is, so the popup never pushes the key bar off the
// bottom.
func (m Model) rerunHeight() int {
	if m.mode != modeRerun {
		return 0
	}
	h := 4
	if m.errText != "" || m.rerunPhase == rerunWorking {
		h++
	}
	return h
}

// paneHeight is what is left for the two panes once the header, the key bar
// and the footer lines have been paid for.
func (m Model) paneHeight() int {
	if m.height <= 0 {
		return 0
	}
	return max(m.height-headerHeight-keyBarHeight-m.errHeight()-m.declinedHeight()-m.rerunHeight(), 1)
}

// body lays the check list and the log pane side by side, the way the diff
// view lays its sidebar and its own pane out.
func (m Model) body() []string {
	h := m.paneHeight()
	left := m.visibleRows()
	right := m.visibleLogRows()
	div := theme.Rule().Render("│")
	lines := make([]string, h)
	for i := range lines {
		l := ""
		if i < len(left) {
			l = left[i]
		}
		r := ""
		if i < len(right) {
			r = right[i]
		}
		lines[i] = fit(l, listWidth) + div + r
	}
	return lines
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
		return []string{clip(theme.Dim().Render(i18n.T("checks.none")), listWidth)}
	}
	var lines []string
	for i, r := range m.order {
		if m.hasHeading(i) {
			lines = append(lines, clip(theme.Heading().Render(m.workflowTitle(r)), listWidth))
		}
		lines = append(lines, m.checkLine(r, i == m.row))
	}
	return lines
}

// hasHeading reports whether the list draws a workflow heading above
// order[i]. A StatusContext belongs to no workflow, and neither does a check
// run an App created: GitHub reports those with a null workflowRun, leaving
// RunID zero and the workflow's name empty. arrange keeps the checks of one
// workflow together, so the row before is enough to tell a new group from a
// continuing one.
func (m Model) hasHeading(i int) bool {
	r := m.order[i]
	if r.Kind != gh.CheckKindRun || r.RunID == 0 {
		return false
	}
	if i == 0 {
		return true
	}
	prev := m.order[i-1]
	return prev.Kind != gh.CheckKindRun || prev.Workflow != r.Workflow
}

// cursorLine is which line of allRows the cursor sits on: its row plus every
// heading drawn at or above it.
func (m Model) cursorLine() int {
	line := m.row
	for i := 0; i <= m.row && i < len(m.order); i++ {
		if m.hasHeading(i) {
			line++
		}
	}
	return line
}

func (m Model) workflowTitle(r gh.CheckRun) string {
	if r.RunNumber > 0 {
		return fmt.Sprintf("%s #%d", r.Workflow, r.RunNumber)
	}
	return r.Workflow
}

// checkLine draws one check: its glyph, its name and, once it has one, how
// long it took. It is highlighted only while the cursor acts on the list --
// once the log pane has focus the row it points at stays plain, the same way
// the diff view's file list does.
func (m Model) checkLine(r gh.CheckRun, cursor bool) string {
	text := "  " + icon.Check(r.State) + " " + r.Name
	if d := r.Duration(); d > 0 {
		text += "  " + minSec(d)
	}
	if cursor && m.pane == paneList {
		return theme.Selected().Render(fit(text, listWidth))
	}
	return theme.Check(r.State).Render(clip(text, listWidth))
}

// minSec formats a duration as m:ss, the same shape the design's mockup
// draws checks with ("2m14s" there is display-only; the on-screen form
// drops the letters).
func minSec(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// logWidth is what is left for the log pane once the list and the divider
// between them have been paid for.
func (m Model) logWidth() int {
	return max(m.width-listWidth-1, 0)
}

// visibleLogRows cuts the log's rows to what is on screen: vertically from
// logRow for paneHeight of them, and horizontally from hscroll for the log
// pane's own width. The cut, not a wrap, is what keeps a long line from
// spilling into extra rows and shifting everything under it down.
func (m Model) visibleLogRows() []string {
	rows := m.logRows()
	end := min(m.logRow+m.paneHeight(), len(rows))
	start := min(m.logRow, len(rows))
	rows = rows[start:end]
	width := m.logWidth()
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = ansi.Cut(r, m.hscroll, m.hscroll+width)
	}
	return lines
}

// logRows builds every line the log pane would draw, ungated by scrolling:
// a heading per step and the step's own lines under it. A continuation
// line has no timestamp of its own (LogLine.Time is zero), so only a
// stamped line gets one drawn in front of it.
func (m Model) logRows() []string {
	if m.logPhase == phaseLoading {
		return []string{m.spin.View() + " " + i18n.T("checks.log_loading")}
	}
	if len(m.log) == 0 {
		if m.logJob != 0 && m.logJob == m.selectedJobID() && m.failedOnly {
			return []string{theme.Dim().Render(i18n.T("checks.log_empty_failed"))}
		}
		return nil
	}
	var lines []string
	last := ""
	first := true
	for _, l := range m.log {
		if first || l.Step != last {
			lines = append(lines, theme.Heading().Render(l.Step))
			last = l.Step
		}
		first = false
		text := l.Text
		if !l.Time.IsZero() {
			text = l.Time.Format("15:04:05") + "  " + text
		}
		lines = append(lines, text)
	}
	return lines
}

// clip cuts s to w display columns. Japanese takes two columns per
// character, so the count is never a byte or a rune count.
func clip(s string, w int) string { return ansi.Truncate(s, w, "…") }

// fit clips s and pads it out to exactly w display columns.
func fit(s string, w int) string {
	s = clip(s, w)
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// singleLine folds an error's body onto one row, the same way the diff
// view's does: a multi-line message would draw as extra visual rows and
// shift everything under it down.
func singleLine(s string) string { return strings.Join(strings.Fields(s), " ") }
