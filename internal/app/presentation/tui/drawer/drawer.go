// Package drawer draws the block under a list: the selected item in full, so
// that it can be read without pressing enter. The Work board and the Repos
// list both end in one, and they are the same block rather than two that
// resemble each other.
package drawer

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

// Height is how many lines the drawer occupies. It is fixed: the drawer sits
// below a list whose length depends on its contents, and one that changed
// height would move the key bar under the user's eyes.
//
// MinColumns is where the drawer folds away. It is two panes side by side,
// and a hundred columns leaves them fifty-eight and thirty-nine.
const (
	Height     = 6
	MinColumns = 100
)

// The drawer is two panes side by side: what the item is on the left, how its
// checks are doing on the right. Its height is fixed, so the panes are cut to
// a budget rather than allowed to grow.
const (
	rows   = Height - 1 // the rule above them takes the other line
	gap    = 3
	checks = 3
)

// Render draws the selected item.
func Render(it domain.WorkItem, width int) []string {
	// The mockup gives the description the larger share; the checks are a
	// short list of short names.
	leftWidth := width * 7 / 12
	rightWidth := width - leftWidth - gap

	left := summaryPane(it, leftWidth)
	right := checksPane(it, rightWidth)

	lines := []string{rule(width)}
	for row := range rows {
		l, r := "", ""
		if row < len(left) {
			l = left[row]
		}
		if row < len(right) {
			r = right[row]
		}
		lines = append(lines, strings.TrimRight(
			layout.Fill(l, leftWidth)+strings.Repeat(" ", gap)+layout.Fill(r, rightWidth), " "))
	}
	return lines
}

// Empty is the drawer with nothing selected: the rule, and the same height
// of blank lines, so that what is drawn under it does not move.
func Empty(width int) []string {
	return append([]string{rule(width)}, make([]string, rows)...)
}

func rule(width int) string {
	return theme.Rule().Render(strings.Repeat("─", max(width, 0)))
}

// summaryPane is the left half: the title, one line of where the item came
// from and what it changes, and the beginning of its body.
func summaryPane(it domain.WorkItem, w int) []string {
	lines := []string{
		clip(theme.Title().Render(it.Title), w),
		clip(metaLine(it, w), w),
	}
	return append(lines, bodyLines(it.Body, w, rows-len(lines))...)
}

// metaLine is the reference, the author, the branches, the size of the change
// and the labels, in the order the mockup puts them. A part with nothing to
// say is left out rather than drawn empty: an account that has been deleted
// carries no login.
func metaLine(it domain.WorkItem, w int) string {
	parts := []string{theme.Dim().Render(fmt.Sprintf("%s #%d", it.Ref.Repo, it.Ref.Number))}
	if it.Author != "" {
		parts = append(parts, theme.Dim().Render("@"+it.Author))
	}
	if it.Head != "" && it.Base != "" {
		parts = append(parts, theme.Accent().Render(it.Head)+
			theme.Dim().Render(" → ")+theme.Accent().Render(it.Base))
	}
	if it.Additions > 0 || it.Deletions > 0 {
		parts = append(parts, theme.Added().Render(fmt.Sprintf("+%d", it.Additions))+
			" "+theme.Removed().Render(fmt.Sprintf("−%d", it.Deletions)))
	}
	// Labels are offered whatever the rest of the line has not already spent,
	// separator included. Measuring anything else lets the clip above cut a
	// badge in half, which reads as a coloured smear rather than a label.
	const sep = " · "
	spent := ansi.StringWidth(strings.Join(parts, sep)) + ansi.StringWidth(sep)
	if b := theme.Badges(it.Labels, w-spent); b != "" {
		parts = append(parts, strings.TrimSpace(b))
	}
	return strings.Join(parts, theme.Dim().Render(sep))
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
// bar. A row only has room for the bar, which says how many passed but not
// which.
func checksPane(it domain.WorkItem, w int) []string {
	// Issues have no checks at all, so they get no pane.
	if it.Ref.Kind == domain.ItemIssue {
		return nil
	}
	c := it.Checks
	if c.Total == 0 {
		return []string{theme.Dim().Render(clip(i18n.T("work.no_checks"), w))}
	}

	// A failure is the reason to look at this list, so failures come first.
	runs := slices.SortedStableFunc(slices.Values(c.Runs), func(a, b domain.CheckRun) int {
		return checkOrder(a.State) - checkOrder(b.State)
	})
	var lines []string
	for _, run := range runs[:min(len(runs), checks)] {
		lines = append(lines, clip(
			theme.Check(run.State).Render(icon.Check(run.State))+" "+run.Name, w))
	}
	if rest := len(runs) - checks; rest > 0 {
		lines = append(lines, theme.Dim().Render(clip(i18n.Tn("work.checks_more", rest), w)))
	}

	summary := i18n.Tf("work.checks_summary", map[string]any{
		"Passed": c.Passed, "Total": c.Total, "Failed": c.Failed, "Running": c.Running,
	})
	return append(lines, theme.Dim().Render(clip(summary, w)))
}

// checkOrder ranks a check by how much it wants attention.
func checkOrder(s domain.CheckState) int {
	switch s {
	case domain.CheckFailure:
		return 0
	case domain.CheckRunning, domain.CheckPending:
		return 1
	default:
		return 2
	}
}

// clip cuts s to w display columns. layout.Clip is a different thing: it
// reserves a column of margin, which a pane already fitted to its own width
// does not want.
func clip(s string, w int) string {
	return ansi.Truncate(s, w, "…")
}
