package work

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// The drawer is two panes side by side: what the item is on the left, how its
// checks are doing on the right (spec 4.1). Its height is fixed — see
// drawerHeight — so the panes are cut to a budget rather than allowed to grow.
const (
	drawerRows   = drawerHeight - 1 // the rule above it takes the other line
	drawerGap    = 3
	drawerChecks = 3
)

// drawer shows the selected card in full, so that it can be read without
// pressing enter.
func (m Model) drawer() []string {
	rule := theme.Rule().Render(strings.Repeat("─", m.width))
	ref, ok := m.SelectedRef()
	if !ok {
		return append([]string{rule}, blankLines(drawerRows)...)
	}
	it := m.work[m.section()][m.row]

	// The mockup gives the description the larger share; the checks are a
	// short list of short names.
	leftWidth := m.width * 7 / 12
	rightWidth := m.width - leftWidth - drawerGap

	left := m.summaryPane(ref, it, leftWidth)
	right := m.checksPane(it, rightWidth)

	lines := []string{rule}
	for row := range drawerRows {
		l, r := "", ""
		if row < len(left) {
			l = left[row]
		}
		if row < len(right) {
			r = right[row]
		}
		lines = append(lines, strings.TrimRight(
			fit(l, leftWidth)+strings.Repeat(" ", drawerGap)+fit(r, rightWidth), " "))
	}
	return lines
}

// summaryPane is the left half: the title, one line of where the item came
// from and what it changes, and the beginning of its body.
func (m Model) summaryPane(ref gh.ItemRef, it gh.WorkItem, w int) []string {
	lines := []string{
		clip(theme.Title().Render(it.Title), w),
		clip(m.metaLine(ref, it), w),
	}
	return append(lines, bodyLines(it.Body, w, drawerRows-len(lines))...)
}

// metaLine is the reference, the branches, the size of the change and the
// labels, in the order the mockup puts them. A part with nothing to say is
// left out rather than drawn empty.
func (m Model) metaLine(ref gh.ItemRef, it gh.WorkItem) string {
	parts := []string{theme.Dim().Render(fmt.Sprintf("%s #%d", ref.Repo, ref.Number))}
	if it.Head != "" && it.Base != "" {
		parts = append(parts, theme.Accent().Render(it.Head)+
			theme.Dim().Render(" → ")+theme.Accent().Render(it.Base))
	}
	if it.Additions > 0 || it.Deletions > 0 {
		parts = append(parts, theme.Added().Render(fmt.Sprintf("+%d", it.Additions))+
			" "+theme.Removed().Render(fmt.Sprintf("−%d", it.Deletions)))
	}
	if b := badges(it.Labels, ansi.StringWidth(strings.Join(parts, " · "))); b != "" {
		parts = append(parts, strings.TrimSpace(b))
	}
	return strings.Join(parts, theme.Dim().Render(" · "))
}

// bodyLines is the beginning of the item's body, wrapped and cut to the lines
// the drawer has left. GitHub bodies run to any length; the drawer is a
// preview, and enter opens the whole thing.
func bodyLines(body string, w, budget int) []string {
	// A GitHub body carries the line endings whoever wrote it used. A stray
	// carriage return inside a drawn line moves the terminal's cursor back to
	// the start of it, which shifts everything after it sideways.
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r", ""))
	if body == "" || budget <= 0 || w <= 0 {
		return nil
	}
	// Blank lines are dropped rather than spent: the preview is short, and a
	// paragraph break costs a line that could have carried words.
	var wrapped []string
	for _, line := range strings.Split(ansi.Wrap(body, w, ""), "\n") {
		if strings.TrimSpace(line) != "" {
			wrapped = append(wrapped, theme.Dim().Render(line))
		}
	}
	if len(wrapped) > budget {
		wrapped = wrapped[:budget]
		wrapped[budget-1] = clip(wrapped[budget-1]+theme.Dim().Render(" …"), w)
	}
	return wrapped
}

// checksPane is the right half: every check by name, then the same ratio as a
// bar. The card only has room for the bar, which says how many passed but not
// which.
func (m Model) checksPane(it gh.WorkItem, w int) []string {
	// Issues have no checks at all, so they get no pane.
	if it.Ref.Kind == gh.ItemIssue {
		return nil
	}
	c := it.Checks
	if c.Total == 0 {
		return []string{theme.Dim().Render(clip(i18n.T("work.no_checks"), w))}
	}

	// A failure is the reason to look at this list, so failures come first.
	runs := slices.SortedStableFunc(slices.Values(c.Runs), func(a, b gh.CheckRun) int {
		return checkOrder(a.State) - checkOrder(b.State)
	})
	var lines []string
	for _, run := range runs[:min(len(runs), drawerChecks)] {
		lines = append(lines, clip(
			theme.Check(run.State).Render(icon.Check(run.State))+" "+run.Name, w))
	}
	if rest := len(runs) - drawerChecks; rest > 0 {
		lines = append(lines, theme.Dim().Render(clip(i18n.Tn("work.checks_more", rest), w)))
	}

	summary := i18n.Tf("work.checks_summary", map[string]any{
		"Passed": c.Passed, "Total": c.Total, "Failed": c.Failed, "Running": c.Running,
	})
	return append(lines, theme.Dim().Render(clip(summary, w)))
}

// checkOrder ranks a check by how much it wants attention.
func checkOrder(s gh.CheckState) int {
	switch s {
	case gh.CheckFailure:
		return 0
	case gh.CheckRunning, gh.CheckPending:
		return 1
	default:
		return 2
	}
}

func blankLines(n int) []string { return make([]string, max(n, 0)) }
