package search

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

// The panes and the result table's columns, in display columns: the filter
// names sit in a ten-column field, with the results beside them.
const (
	filterPaneWidth = 30
	paneRule        = 1
	filterNameWidth = 10

	// minPaneWidth is where the filter pane folds away: under it there is
	// not enough room for both an eight-row field list and a readable table.
	minPaneWidth = 100

	stateColumn  = 2
	repoColumn   = 16
	numberColumn = 6
	ageColumn    = 8

	// promptCols is the "> " textinput draws in front of what is typed
	// (internal/tui/dialog uses the same figure for the same reason).
	promptCols = 2

	// cursorCol is the extra column textinput reserves after the typed text
	// for its cursor cell, on top of promptCols -- it is there even on an
	// empty value, so a field sized to promptCols alone still overflows by
	// one column once its own SetWidth is added to a row built around it.
	cursorCol = 1

	// queryRowHeight is the query line and the blank line under it;
	// footerHeight is the blank line and the key bar.
	queryRowHeight = 2
	footerHeight   = 2

	// searchCap is what one search asks GitHub for. A page of exactly this
	// many may have been cut short.
	searchCap = 50
)

func (m Model) View() string {
	if m.width <= 0 {
		// The root routes keys to this tab before the first size arrives, so
		// whatever a key has just changed -- a typed query, a picked saved
		// one -- must show without a width to lay the rest of the screen out
		// against.
		if m.mode == modePicker {
			return m.pickerView()
		}
		return m.queryRow()
	}
	if m.mode == modePicker {
		return m.pickerView() + "\n" + m.keyBar()
	}
	lines := []string{m.queryRow(), ""}
	if m.paneCols() > 0 {
		lines = append(lines, layout.JoinPanes(m.filterPane(), m.resultPane(), filterPaneWidth)...)
	} else {
		lines = append(lines, m.resultPane()...)
	}
	lines = append(lines, "")
	if m.notice != "" {
		lines = append(lines, theme.Error().Render(layout.Notice(m.notice, m.width)))
	}
	return strings.Join(append(lines, m.keyBar()), "\n")
}

// paneCols is how much of the terminal the filter pane takes, rule
// included. Zero means it is folded away and the results take the whole
// width.
func (m Model) paneCols() int {
	if m.width < minPaneWidth {
		return 0
	}
	return filterPaneWidth + paneRule
}

// resultWidth is what is left for the result pane once the filter pane has
// taken its share.
func (m Model) resultWidth() int {
	return max(m.width-m.paneCols(), 1)
}

// queryRow is the raw query above the panes, with the count of what it
// found at the right edge. While the raw editor is open it draws the field
// itself instead, with no count: what it will find is not known until enter
// runs it.
func (m Model) queryRow() string {
	switch m.mode {
	case modeRaw:
		line := "q " + m.input.View()
		if m.width <= 0 {
			return line
		}
		return layout.Clip(line, m.width)
	case modeName:
		line := i18n.T("search.save_prompt") + " " + m.input.View()
		if m.width <= 0 {
			return line
		}
		return layout.Clip(line, m.width)
	}
	query := m.query()
	left := "q " + theme.Dim().Render(query)
	if m.width <= 0 {
		return left
	}
	count := m.countText()
	room := max(m.width-ansi.StringWidth(count), 0)
	return layout.Pad(left, room) + count
}

// countText is what the query row says it found: the exact count, or
// search.result_count_capped once a full page may have been cut short
// ("50+ results" is not a real quantity to pluralize on, so it has its own
// message rather than one built from search.result_count's pieces).
func (m Model) countText() string {
	if len(m.items) >= searchCap {
		return i18n.Tf("search.result_count_capped", map[string]any{"Count": searchCap})
	}
	return i18n.Tn("search.result_count", len(m.items))
}

// countBadge is the same count, without the translated words: the result
// pane already names itself "Results", so repeating "results" there would
// read as "Results 2 results". "50+" needs no catalog lookup either, since
// digits and "+" mean the same thing in every language.
func (m Model) countBadge() string {
	if len(m.items) >= searchCap {
		return fmt.Sprintf("%d+", searchCap)
	}
	return strconv.Itoa(len(m.items))
}

func (m Model) keyBar() string {
	return theme.Dim().Render(layout.FitKeyBar(m.footerHints(), m.width))
}

// footerHints is the key bar for whichever pane has the focus, most
// important first. h/l cross between them, so each side names the other.
//
// While a field or the raw editor is open, every other key types into it
// (.claude/rules/tui.md: mode, not a bool, decides what is drawn), so the
// bar names only the way out and the way to confirm. esc leads because
// FitKeyBar never drops the first hint: it is the only way out.
func (m Model) footerHints() []string {
	if m.mode == modePicker {
		return []string{i18n.T("footer.search.cancel"), i18n.T("footer.search.apply"), i18n.T("footer.search.remove")}
	}
	if m.Capturing() {
		return []string{i18n.T("footer.search.cancel"), i18n.T("footer.search.apply")}
	}
	if m.pane == paneResults {
		hints := []string{i18n.T("footer.search.move"), i18n.T("footer.search.open")}
		if m.paneCols() > 0 {
			hints = append(hints, i18n.T("footer.search.back"))
		}
		return append(hints,
			i18n.T("footer.search.diff"),
			i18n.T("footer.search.web"),
			i18n.T("footer.search.raw"),
			i18n.T("footer.search.save"),
			i18n.T("footer.search.refresh"),
			i18n.T("footer.search.quit"),
			i18n.T("footer.search.open_saved"))
	}
	hints := []string{
		i18n.T("footer.search.field"),
		i18n.T("footer.search.cycle"),
		i18n.T("footer.search.edit_field"),
	}
	if m.paneCols() > 0 {
		hints = append(hints, i18n.T("footer.search.results"))
	}
	return append(hints,
		i18n.T("footer.search.raw"),
		i18n.T("footer.search.save"),
		i18n.T("footer.search.refresh"),
		i18n.T("footer.search.quit"),
		i18n.T("footer.search.open_saved"))
}

// filterPane draws the eight filters, one per row: its name in a fixed
// field, then its value. The row under the pane's own cursor is reversed,
// and the whole pane is dimmed while the raw editor holds the query instead:
// the filters are not rebuilt from what is typed there. Under them, the
// repository's labels or authors are offered as chips while the cursor sits
// on the filter they belong to.
func (m Model) filterPane() []string {
	lines := []string{theme.Heading().Render(i18n.T("search.filters"))}
	for id := FilterType; id < filterCount; id++ {
		lines = append(lines, m.filterRow(id))
	}
	if chips := m.candidateChips(); chips != "" {
		lines = append(lines, "", theme.Heading().Render(i18n.T("search.candidates")), chips)
	}
	return lines
}

// candidateChips is the chip row for whichever filter the cursor is on, or
// empty while the cursor is elsewhere, the raw editor is open, nothing has
// arrived yet for the named repository, or repo: has since changed or been
// cleared: "no repo:, no candidates" applies to what is drawn as much as to
// what is fetched, so a cached answer for a repository that is no longer
// named must not still be on screen.
func (m Model) candidateChips() string {
	if m.mode == modeRaw || m.pane != paneFilters {
		return ""
	}
	repo := m.filters.Value(FilterRepo)
	switch m.cursor {
	case FilterLabel:
		if m.labelCandidatesRepo != repo {
			return ""
		}
		return labelChips(m.labelCandidates, filterPaneWidth)
	case FilterAuthor:
		if m.authorCandidatesRepo != repo {
			return ""
		}
		return authorChips(m.authorCandidates, filterPaneWidth)
	}
	return ""
}

// labelChips draws a repository's labels as chips in the colour GitHub gave
// them, the same "drop whatever does not fit" rule internal/tui/repo's
// badges() uses for the same reason: a wrapped chip row would push the rest
// of the pane down by an amount that depends on the repository.
func labelChips(labels []gh.Label, room int) string {
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

// authorChips draws logins the same way. A login has no colour of its own;
// theme.Badge falls back to its muted style for an unusable hex.
func authorChips(users []string, room int) string {
	var b strings.Builder
	for _, u := range users {
		text := " " + u + " "
		cost := ansi.StringWidth(text) + 1
		if cost > room {
			break
		}
		b.WriteString(" " + theme.Badge("").Render(text))
		room -= cost
	}
	return b.String()
}

func (m Model) filterRow(id FilterID) string {
	label := layout.Pad(theme.Dim().Render(i18n.T(filterLabelID(id))), filterNameWidth)
	if m.mode == modeField && id == m.cursor {
		return layout.Clip(label+m.input.View(), filterPaneWidth)
	}

	value := m.filters.Value(id)
	if value == "" {
		value = i18n.T("search.unset")
	}
	line := label + value

	if m.mode == modeRaw {
		return theme.Dim().Render(line)
	}
	if m.pane == paneFilters && id == m.cursor {
		return theme.Selected().Render(line)
	}
	return line
}

// filterLabelID names the message ID for one filter's label, which is
// GitHub's own qualifier name and stays the same in every language
// (.claude/rules/tui.md).
func filterLabelID(id FilterID) string {
	return [filterCount]string{
		FilterType:   "search.filter.type",
		FilterState:  "search.filter.state",
		FilterOrg:    "search.filter.org",
		FilterRepo:   "search.filter.repo",
		FilterAuthor: "search.filter.author",
		FilterLabel:  "search.filter.label",
		FilterReview: "search.filter.review",
		FilterSort:   "search.filter.sort",
	}[id]
}

// resultPane draws the heading and the table of items, or what stands in
// for it while there is nothing to show.
func (m Model) resultPane() []string {
	width := m.resultWidth()
	heading := theme.Heading().Render(i18n.T("search.results")) + " " + m.countBadge()
	lines := []string{layout.Clip(heading, width)}

	if m.loading {
		return append(lines, layout.Clip(m.spin.View()+" "+i18n.T("common.loading"), width))
	}
	if len(m.items) == 0 {
		return append(lines, theme.Dim().Render(layout.Clip(i18n.T("search.no_results"), width)))
	}

	rows := m.resultRows()
	first := m.resultWindow(rows)
	for i := first; i < min(first+rows, len(m.items)); i++ {
		lines = append(lines, m.resultRow(i, width))
	}
	return lines
}

// resultRows is how many item rows fit under the result pane's heading.
// Search has neither Repos' sub-tabs nor its summary block, so its budget
// is only the query row and the key bar.
func (m Model) resultRows() int {
	if m.height <= 0 {
		return len(m.items)
	}
	rows := m.height - queryRowHeight - footerHeight - 1 // 1: the pane's own heading
	if m.notice != "" {
		rows--
	}
	return max(rows, 1)
}

// resultWindow is the first item drawn, chosen to keep the cursor in view.
func (m Model) resultWindow(rows int) int {
	if m.sel < rows {
		return 0
	}
	return m.sel - rows + 1
}

// resultRow draws one line of the table: state, repository (owner
// dropped), number, title, and relative age.
func (m Model) resultRow(i int, width int) string {
	item := m.items[i]

	state := theme.Dim().Render(icon.Issue())
	if item.Ref.Kind == gh.ItemPR {
		state = theme.Review(item.Review, item.IsDraft).Render(icon.Review(item.Review, item.IsDraft))
	}
	_, name, _ := gh.SplitRepo(item.Ref.Repo)
	number := "#" + strconv.Itoa(item.Ref.Number)
	age := i18n.RelTime(m.fetchedAt, item.UpdatedAt)

	titleWidth := max(width-stateColumn-repoColumn-numberColumn-ageColumn, 1)
	line := layout.Pad(state, stateColumn) +
		layout.Pad(name, repoColumn) +
		layout.Pad(theme.Dim().Render(number), numberColumn) +
		layout.Pad(item.Title, titleWidth) +
		layout.Right(theme.Dim().Render(age), ageColumn)

	if i == m.sel {
		return theme.Selected().Render(layout.Clip(line, width))
	}
	return layout.Clip(line, width)
}
