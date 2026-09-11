package repo

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// The right pane is a table with fixed columns, so that the eye can run down
// one of them. The title takes whatever the others leave.
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
	if m.mode == modeAdd {
		return m.dlg.View() + "\n" +
			theme.Dim().Render(layout.FitKeyBar(addDialogHints(), m.width))
	}
	lines := append(m.header(), m.body()...)
	if m.itemCount() > 0 && !m.loading[m.tab] {
		lines = append(lines, m.summary()...)
	}
	if m.sidebarCols() > 0 {
		lines = joinPanes(m.sidebar(), lines, sidebarWidth)
	}
	lines = append(lines, "")
	if n := m.noticeLine(); n != "" {
		lines = append(lines, n)
	}
	return strings.Join(append(lines, m.keyBar()), "\n")
}

// noticeLine is what went wrong, above the key bar. It spans the whole
// terminal rather than the table's own width: the sidebar is not what
// failed, and a message cut to the narrower pane would lose what GitHub said.
func (m Model) noticeLine() string {
	n := m.notice[m.tab]
	if n.text == "" {
		return ""
	}
	return theme.Error().Render(
		layout.Notice(i18n.T(n.kind.prefixID())+" · "+n.text, m.width))
}

func (m Model) keyBar() string {
	return theme.Dim().Render(layout.FitKeyBar(m.footerHints(), m.width))
}

// footerHints is the list's key bar, most important first: FitKeyBar drops
// from the end when the terminal is too narrow for all of them. quit comes
// before the item-specific shortcuts: it is how the user leaves octoscope,
// and losing it at a narrow width would leave no visible way out.
func (m Model) footerHints() []string {
	return []string{
		i18n.T("footer.list.move"),
		i18n.T("footer.list.open"),
		i18n.T("footer.list.pane"),
		i18n.T("footer.list.kind"),
		i18n.T("footer.list.quit"),
		i18n.T("footer.list.refresh"),
		i18n.T("footer.list.add"),
		i18n.T("footer.list.remove"),
		i18n.T("footer.list.seed"),
		i18n.T("footer.list.diff"),
		i18n.T("footer.list.checks"),
		i18n.T("footer.list.web"),
	}
}

// addDialogHints is the dialog's key bar, most important first. esc leads
// because FitKeyBar never drops the first hint: it is the only way out of
// the popup.
func addDialogHints() []string {
	return []string{
		i18n.T("footer.dialog.close"),
		i18n.T("footer.dialog.add"),
		i18n.T("footer.dialog.candidates"),
	}
}

// header is the repository and the sub-tab row, with the count of each tab
// beside its name so the other one can be judged without switching to it.
func (m Model) header() []string {
	name := m.selectedRepo()
	if name == "" {
		name = i18n.T("app.name")
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
		clip(theme.Title().Render(name), m.bodyWidth()),
		"",
		clip(strings.Join(labels, subTabGap), m.bodyWidth()),
		"",
	}
}

// body is the table, or what stands in for it while there is nothing to draw.
func (m Model) body() []string {
	if m.loading[m.tab] {
		return []string{clip(m.spin.View()+" "+i18n.T("common.loading"), m.bodyWidth())}
	}
	if m.itemCount() == 0 {
		// The empty-tab text would report an outage as "no open pull
		// requests". Only a tab that has never been answered is empty in the
		// first place: a failed refetch keeps the rows it had.
		if !m.loaded[m.tab] && m.notice[m.tab].kind == noticeFetch && m.notice[m.tab].text != "" {
			return []string{theme.Error().Render(clip(i18n.T("notice.fetch_failed"), m.bodyWidth()))}
		}
		// An empty list is not an answer until the lookup that names the
		// working directory's repository has given one: see
		// Model.currentSettled.
		if len(m.rows) == 0 && !m.currentSettled {
			return []string{clip(m.spin.View()+" "+i18n.T("common.loading"), m.bodyWidth())}
		}
		if len(m.rows) == 0 {
			return []string{
				theme.Dim().Render(clip(i18n.T("repos.none"), m.bodyWidth())),
				"",
				theme.Accent().Render(clip(i18n.T("repos.seed_hint"), m.bodyWidth())),
			}
		}
		empty := i18n.T("list.no_open_prs")
		if m.tab == tabIssues {
			empty = i18n.T("list.no_open_issues")
		}
		return []string{theme.Dim().Render(clip(empty, m.bodyWidth()))}
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
	rows := m.height - listTop - summaryHeight - footerHeight
	if m.notice[m.tab].text != "" {
		rows--
	}
	return max(rows, 1)
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

	titleWidth := max(m.bodyWidth()-stateColumn-numberColumn-checksColumn-ageColumn, 1)
	title += badges(labels, titleWidth-ansi.StringWidth(title)-1)
	line := pad(state, stateColumn) +
		pad(theme.Dim().Render(number), numberColumn) +
		pad(title, titleWidth) +
		pad(checks, checksColumn) +
		right(theme.Dim().Render(age), ageColumn)

	if i == m.cursors[m.tab] {
		return theme.Selected().Render(clip(line, m.bodyWidth()))
	}
	return clip(line, m.bodyWidth())
}

// summary is the block under the table: what the selected item changes, and
// how its checks are doing.
func (m Model) summary() []string {
	lines := []string{theme.Rule().Render(strings.Repeat("─", m.bodyWidth()))}
	if m.tab == tabIssues {
		issue := m.issues[m.cursors[tabIssues]]
		return fill(append(lines, clip(theme.Dim().Render(
			fmt.Sprintf("@%s · %s", issue.Author.Login, i18n.DateTime(issue.UpdatedAt))),
			m.bodyWidth())), summaryHeight)
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
	lines = append(lines, clip(strings.Join(parts, theme.Dim().Render(" · ")), m.bodyWidth()))

	// The rest of the block lists the checks by name: the bar on the row says
	// how many passed, but not which.
	if pr.Checks.Total == 0 {
		return fill(append(lines,
			theme.Dim().Render(clip(i18n.T("work.no_checks"), m.bodyWidth()))), summaryHeight)
	}
	for _, run := range pr.Checks.Runs {
		if len(lines) >= summaryHeight {
			break
		}
		lines = append(lines, clip(
			theme.Check(run.State).Render(icon.Check(run.State))+" "+run.Name, m.bodyWidth()))
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
