package repo

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/drawer"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

// The right pane is a table with fixed columns, so that the eye can run down
// one of them. The title takes whatever the others leave.
const (
	stateColumn  = 2
	numberColumn = 6
	checksColumn = 8
	ageColumn    = 8

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
	lines := append(m.header(), m.tableLines()...)
	if m.sidebarCols() > 0 {
		lines = layout.JoinPanes(m.sidebar(), lines, sidebarWidth)
	}
	// The drawer spans the terminal, sidebar included, so it is laid under
	// both panes rather than inside the right one.
	if m.drawerShown() {
		lines = append(lines, m.drawerLines()...)
	}
	lines = append(lines, "")
	if n := m.noticeLine(); n != "" {
		lines = append(lines, n)
	}
	return strings.Join(append(lines, m.keyBar()), "\n")
}

// tableLines is the table, padded out to the rows it was budgeted so that
// what is drawn under it lands on the same line however few rows there were.
//
// The padding happens here rather than after JoinPanes so that the rule
// beside the sidebar runs to the bottom of the screen rather than stopping
// where the rows ran out.
func (m Model) tableLines() []string {
	if m.height <= 0 {
		return m.body() // no budget yet: draw what there is
	}
	return layout.PadLines(m.body(), m.visibleRows())
}

// drawerShown folds the drawer away on a narrow terminal, as the board does:
// it is two panes side by side, and under a hundred columns there is no room
// for both. The table takes the rows back.
func (m Model) drawerShown() bool { return m.width >= drawer.MinColumns }

// drawerLines is the drawer for the selected item. A tab that is loading or
// has nothing in it still spends the same height on it, or the key bar would
// move as the rows arrived.
func (m Model) drawerLines() []string {
	it, ok := m.selectedItem()
	if !ok {
		return drawer.Empty(m.width)
	}
	return drawer.Render(it, m.width)
}

// selectedItem is the row under the cursor as the drawer wants it. Both tabs
// draw the same block, so the pull request and the issue are put into the
// shape the board already had for them. WorkItem.Body is the preview's plain
// text everywhere it is drawn, so the listed item's BodyText goes there and
// its markdown does not.
func (m Model) selectedItem() (domain.WorkItem, bool) {
	if m.itemCount() == 0 || m.loading[m.tab] {
		return domain.WorkItem{}, false
	}
	repo := m.selectedRepo()
	if m.tab == tabIssues {
		issue := m.issues[m.cursors[tabIssues]]
		return domain.WorkItem{
			Ref:       domain.ItemRef{Kind: domain.ItemIssue, Repo: repo, Number: issue.Ref.Number},
			Title:     issue.Title,
			Body:      issue.BodyText,
			Author:    issue.Author.Login,
			State:     issue.State,
			Labels:    issue.Labels,
			UpdatedAt: issue.UpdatedAt,
			URL:       issue.URL,
		}, true
	}
	pr := m.prs[m.cursors[tabPRs]]
	return domain.WorkItem{
		Ref:       domain.ItemRef{Kind: domain.ItemPR, Repo: repo, Number: pr.Ref.Number},
		Title:     pr.Title,
		Body:      pr.BodyText,
		Author:    pr.Author.Login,
		IsDraft:   pr.Change.IsDraft,
		State:     pr.State,
		Labels:    pr.Labels,
		Review:    pr.Change.Review,
		Head:      pr.Change.Head,
		Base:      pr.Change.Base,
		Additions: pr.Change.Additions,
		Deletions: pr.Change.Deletions,
		Checks:    pr.Change.Checks,
		UpdatedAt: pr.UpdatedAt,
		URL:       pr.URL,
	}, true
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
		layout.Clip(theme.Title().Render(name), m.bodyWidth()),
		"",
		layout.Clip(strings.Join(labels, subTabGap), m.bodyWidth()),
		"",
	}
}

// body is the table, or what stands in for it while there is nothing to draw.
func (m Model) body() []string {
	if m.loading[m.tab] {
		return []string{layout.Clip(m.spin.View()+" "+i18n.T("common.loading"), m.bodyWidth())}
	}
	if m.itemCount() == 0 {
		// The empty-tab text would report an outage as "no open pull
		// requests". Only a tab that has never been answered is empty in the
		// first place: a failed refetch keeps the rows it had.
		if !m.loaded[m.tab] && m.notice[m.tab].kind == noticeFetch && m.notice[m.tab].text != "" {
			return []string{theme.Error().Render(layout.Clip(i18n.T("notice.fetch_failed"), m.bodyWidth()))}
		}
		// An empty list is not an answer until the lookup that names the
		// working directory's repository has given one: see
		// Model.currentSettled.
		if len(m.rows) == 0 && !m.currentSettled {
			return []string{layout.Clip(m.spin.View()+" "+i18n.T("common.loading"), m.bodyWidth())}
		}
		if len(m.rows) == 0 {
			return []string{
				theme.Dim().Render(layout.Clip(i18n.T("repos.none"), m.bodyWidth())),
				"",
				theme.Accent().Render(layout.Clip(i18n.T("repos.seed_hint"), m.bodyWidth())),
			}
		}
		empty := i18n.T("list.no_open_prs")
		if m.tab == tabIssues {
			empty = i18n.T("list.no_open_issues")
		}
		return []string{theme.Dim().Render(layout.Clip(empty, m.bodyWidth()))}
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
	rows := m.height - listTop - footerHeight
	if m.drawerShown() {
		rows -= drawer.Height
	}
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
		labels                            []domain.Label
	)
	if m.tab == tabPRs {
		pr := m.prs[i]
		ch := pr.Change
		state = theme.Review(ch.Review, ch.IsDraft).Render(icon.Review(ch.Review, ch.IsDraft))
		number, title, labels = "#"+strconv.Itoa(pr.Ref.Number), pr.Title, pr.Labels
		checks = checksBar(ch.Checks)
		age = i18n.RelTime(m.fetchedAt[tabPRs], pr.UpdatedAt)
	} else {
		issue := m.issues[i]
		state = theme.Issue().Render(icon.Issue())
		number, title, labels = "#"+strconv.Itoa(issue.Ref.Number), issue.Title, issue.Labels
		age = i18n.RelTime(m.fetchedAt[tabIssues], issue.UpdatedAt)
	}

	titleWidth := max(m.bodyWidth()-stateColumn-numberColumn-checksColumn-ageColumn, 1)
	title += theme.Badges(labels, titleWidth-ansi.StringWidth(title)-1)
	line := layout.Pad(state, stateColumn) +
		layout.Pad(theme.Dim().Render(number), numberColumn) +
		layout.Pad(title, titleWidth) +
		layout.Pad(checks, checksColumn) +
		layout.Right(theme.Dim().Render(age), ageColumn)

	if i == m.cursors[m.tab] {
		return theme.SelectedLine(layout.Fill(line, m.bodyWidth()))
	}
	return layout.Clip(line, m.bodyWidth())
}

// checksBar colours the two halves of the bar apart: what has passed takes the
// colour of the roll-up, what has not stays muted.
func checksBar(c domain.Checks) string {
	done, rest := icon.ChecksBar(c)
	if done == "" && rest == "" {
		return ""
	}
	return theme.Check(c.State).Render(done) + theme.Dim().Render(rest)
}

// subTabLabels names the sub-tabs in display order. The hit-test walks the
// same list the row is drawn from, so a rename cannot move one without moving
// the other.
func subTabLabels() []string {
	return []string{i18n.T("list.tab_prs"), i18n.T("list.tab_issues")}
}
