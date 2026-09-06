package repo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// The right pane is a table with fixed columns, so that the eye can run down
// one of them (spec 4.2). The title takes whatever the others leave.
const (
	stateColumn  = 2
	numberColumn = 6
	checksColumn = 8
	ageColumn    = 8

	// summaryHeight is the block under the table: what the selected item
	// changes, then its checks. Fixed, so the table above it does not move as
	// the selection travels down it.
	summaryHeight = 4

	// footerHeight is the blank line and the key bar.
	footerHeight = 2
)

func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}
	lines := append(m.header(), m.body()...)
	if m.itemCount() > 0 && !m.loading[m.tab] {
		lines = append(lines, m.summary()...)
	}
	return strings.Join(append(lines, "", m.keyBar()), "\n")
}

func (m Model) keyBar() string {
	return theme.Dim().Render(clip(i18n.T("footer.list"), m.width))
}

// header is the repository and the sub-tab row, with the count of each tab
// beside its name so the other one can be judged without switching to it.
func (m Model) header() []string {
	name := i18n.T("app.name")
	if m.repoName != "" {
		name = m.repoName
	}

	labels := subTabLabels()
	counts := []int{len(m.prs), len(m.issues)}
	for i, label := range labels {
		count := ""
		if m.loaded[tabID(i)] {
			count = " " + strconv.Itoa(counts[i])
		}
		if tabID(i) == m.tab {
			labels[i] = theme.ActiveTab().Render(label) + theme.Accent().Render(count)
		} else {
			labels[i] = theme.Dim().Render(label + count)
		}
	}
	return []string{
		clip(theme.Title().Render(name), m.width),
		"",
		clip(strings.Join(labels, subTabGap), m.width),
		"",
	}
}

// body is the table, or what stands in for it while there is nothing to draw.
func (m Model) body() []string {
	if m.loading[m.tab] {
		return []string{clip(m.spin.View()+" "+i18n.T("common.loading"), m.width)}
	}
	if m.itemCount() == 0 {
		empty := i18n.T("list.no_open_prs")
		if m.tab == tabIssues {
			empty = i18n.T("list.no_open_issues")
		}
		return []string{theme.Dim().Render(clip(empty, m.width))}
	}

	rows := m.visibleRows()
	first := m.rowWindow(rows)
	lines := make([]string, 0, rows)
	for i := first; i < min(first+rows, m.itemCount()); i++ {
		lines = append(lines, m.row(i))
	}
	return lines
}

// visibleRows is how many rows fit between the header and everything below it.
func (m Model) visibleRows() int {
	if m.height <= 0 {
		return m.itemCount() // no budget yet: draw them all
	}
	return max(m.height-listTop-summaryHeight-footerHeight, 1)
}

// rowWindow is the first row drawn, chosen to keep the cursor in view.
func (m Model) rowWindow(rows int) int {
	if m.cursors[m.tab] < rows {
		return 0
	}
	return m.cursors[m.tab] - rows + 1
}

// row draws one line of the table. Every field but the title has a fixed
// width, so the columns line up down the page whatever the titles do.
func (m Model) row(i int) string {
	var (
		state, number, title, checks, age string
		labels                            []gh.Label
	)
	if m.tab == tabPRs {
		pr := m.prs[i]
		state = theme.Review(pr.Review, pr.IsDraft).Render(icon.Review(pr.Review, pr.IsDraft))
		number, title, labels = "#"+strconv.Itoa(pr.Number), pr.Title, pr.Labels
		checks = checksBar(pr.Checks)
		age = i18n.RelTime(m.fetchedAt[tabPRs], pr.UpdatedAt)
	} else {
		issue := m.issues[i]
		state = theme.Dim().Render(icon.Issue())
		number, title, labels = "#"+strconv.Itoa(issue.Number), issue.Title, issue.Labels
		age = i18n.RelTime(m.fetchedAt[tabIssues], issue.UpdatedAt)
	}

	titleWidth := max(m.width-stateColumn-numberColumn-checksColumn-ageColumn, 1)
	title += badges(labels, titleWidth-ansi.StringWidth(title)-1)
	line := pad(state, stateColumn) +
		pad(theme.Dim().Render(number), numberColumn) +
		pad(title, titleWidth) +
		pad(checks, checksColumn) +
		right(theme.Dim().Render(age), ageColumn)

	if i == m.cursors[m.tab] {
		return theme.Selected().Render(clip(line, m.width))
	}
	return clip(line, m.width)
}

// summary is the block under the table: what the selected item changes, and
// how its checks are doing (spec 4.2).
func (m Model) summary() []string {
	lines := []string{theme.Rule().Render(strings.Repeat("─", m.width))}
	if m.tab == tabIssues {
		issue := m.issues[m.cursors[tabIssues]]
		return fill(append(lines, clip(theme.Dim().Render(
			fmt.Sprintf("@%s · %s", issue.Author.Login, i18n.DateTime(issue.UpdatedAt))),
			m.width)), summaryHeight)
	}

	pr := m.prs[m.cursors[tabPRs]]
	parts := []string{theme.Dim().Render("@" + pr.Author.Login)}
	if pr.Head != "" && pr.Base != "" {
		parts = append(parts, theme.Accent().Render(pr.Head)+
			theme.Dim().Render(" → ")+theme.Accent().Render(pr.Base))
	}
	if pr.Additions > 0 || pr.Deletions > 0 {
		parts = append(parts, theme.Added().Render(fmt.Sprintf("+%d", pr.Additions))+
			" "+theme.Removed().Render(fmt.Sprintf("−%d", pr.Deletions)))
	}
	lines = append(lines, clip(strings.Join(parts, theme.Dim().Render(" · ")), m.width))

	// The rest of the block lists the checks by name: the bar on the row says
	// how many passed, but not which.
	if pr.Checks.Total == 0 {
		return fill(append(lines,
			theme.Dim().Render(clip(i18n.T("work.no_checks"), m.width))), summaryHeight)
	}
	for _, run := range pr.Checks.Runs {
		if len(lines) >= summaryHeight {
			break
		}
		lines = append(lines, clip(
			theme.Check(run.State).Render(icon.Check(run.State))+" "+run.Name, m.width))
	}
	return fill(lines, summaryHeight)
}

// checksBar colours the two halves of the bar apart: what has passed takes the
// colour of the roll-up, what has not stays muted.
func checksBar(c gh.Checks) string {
	done, rest := icon.ChecksBar(c)
	if done == "" && rest == "" {
		return ""
	}
	return theme.Check(c.State).Render(done) + theme.Dim().Render(rest)
}

// badges draws the labels that fit in room columns, in the colours GitHub
// gave them. A label that would be cut in half is left out altogether.
func badges(labels []gh.Label, room int) string {
	var b strings.Builder
	for _, l := range labels {
		text := " " + l.Name + " "
		cost := ansi.StringWidth(text) + 1
		if cost > room {
			break
		}
		b.WriteString(" " + theme.Badge(l.Color).Render(text))
		room -= cost
	}
	return b.String()
}

// subTabLabels names the sub-tabs in display order. The hit-test walks the
// same list the row is drawn from, so a rename cannot move one without moving
// the other.
func subTabLabels() []string {
	return []string{i18n.T("list.tab_prs"), i18n.T("list.tab_issues")}
}

// fill pads a block out to exactly n lines, so what is drawn under it stays
// where it was.
func fill(lines []string, n int) []string {
	for len(lines) < n {
		lines = append(lines, "")
	}
	return lines[:n]
}

// clip cuts s to w display columns. Japanese takes two columns per character,
// so the count is never a byte or a rune count.
func clip(s string, w int) string { return ansi.Truncate(s, w, "…") }

// pad clips s to one column short of w and pads it out, so two fields never
// run into each other.
func pad(s string, w int) string {
	s = clip(s, max(w-1, 0))
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// right pads s on the left instead, so a column of ages ends flush.
func right(s string, w int) string {
	s = clip(s, w)
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}
