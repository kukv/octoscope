package work

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/theme"
)

const (
	columnGap         = 2
	drawerMinColumns  = 100
	singleColumnBelow = 60

	// gutter is the column the cursor marker lives in. Unselected cards keep
	// it blank rather than closing it up, so card text does not jump sideways
	// as the cursor moves.
	gutter = "  "
)

// sectionTitleIDs maps a column to its heading in the catalog.
var sectionTitleIDs = map[gh.WorkSection]string{
	gh.SectionReviewRequested: "work.review_requested",
	gh.SectionYourPRs:         "work.your_prs",
	gh.SectionAssigned:        "work.assigned",
	gh.SectionMentioned:       "work.mentioned",
}

func (m Model) View() string {
	// Before the first WindowSizeMsg there is no width to lay anything out in,
	// and every column budget below would go negative.
	if m.width <= 0 {
		return ""
	}
	if m.loading {
		return clip(m.spin.View()+" "+i18n.T("common.loading"), m.width)
	}

	var lines []string
	sections := m.visibleSections()
	if len(sections) < m.columns() {
		position := i18n.Tf("work.column_position", map[string]any{
			"Index": m.col + 1,
			"Total": m.columns(),
		})
		lines = append(lines, theme.Dim().Render(clip(position, m.width)), "")
	}
	lines = append(lines, m.board(sections)...)
	if m.width >= drawerMinColumns {
		lines = append(lines, m.drawer()...)
	}
	lines = append(lines, "", theme.Dim().Render(clip(i18n.T("footer.work"), m.width)))
	return strings.Join(lines, "\n")
}

// board lays the columns side by side. Every cell is padded to the column
// width first, so the columns stay aligned however many cards each one holds.
func (m Model) board(sections []gh.WorkSection) []string {
	w := m.columnWidth(len(sections))
	columns := make([][]string, len(sections))
	height := 0
	for i, s := range sections {
		columns[i] = m.columnLines(s, w)
		height = max(height, len(columns[i]))
	}

	gap := strings.Repeat(" ", columnGap)
	blank := strings.Repeat(" ", w)
	lines := make([]string, height)
	for row := range height {
		cells := make([]string, len(columns))
		for i, column := range columns {
			cells[i] = blank
			if row < len(column) {
				cells[i] = column[row]
			}
		}
		lines[row] = strings.TrimRight(strings.Join(cells, gap), " ")
	}
	return lines
}

func (m Model) columnLines(s gh.WorkSection, w int) []string {
	// The heading text is indented by the cursor gutter so it starts in the
	// same column as the card titles below it. The rule is not: it marks how
	// far the column reaches, which is the whole of its width.
	lines := []string{
		theme.Heading().Render(fit(gutter+i18n.T(sectionTitleIDs[s]), w)),
		fit(strings.Repeat("─", w), w),
	}
	items := m.work[s]
	if len(items) == 0 {
		return append(lines, theme.Dim().Render(fit(gutter+i18n.T("work.empty_column"), w)))
	}
	for i, it := range items {
		lines = append(lines, cardLines(it, w, s == m.section() && i == m.row, m.fetchedAt)...)
	}
	return lines
}

// cardLines draws one card in two lines: what it is on the first, where it
// lives and how it is doing on the second (spec 4.1).
func cardLines(it gh.WorkItem, w int, selected bool, now time.Time) []string {
	return []string{cardTitle(it, w, selected), cardMeta(it, w, now)}
}

// cardTitle is the cursor gutter, the state marker and the title. The pieces
// are styled one at a time rather than as a whole line: a style applied over
// a coloured marker would end at that marker's own reset.
func cardTitle(it gh.WorkItem, w int, selected bool) string {
	marker := gutter
	title := it.Title
	if selected {
		marker = theme.Cursor().Render("▸ ")
		title = theme.Cursor().Render(title)
	}
	head := marker + stateMarker(it) + " " + title
	// Badges are added only while the whole title still fits: a label is worth
	// less than the title it would push off the card.
	if room := w - ansi.StringWidth(head); room > 0 {
		head += badges(it.Labels, room)
	}
	return fit(head, w)
}

// badges draws the labels that fit in room columns, in the colours GitHub
// gave them (spec §4.5). A label that would be cut in half is left out
// altogether rather than shown as a coloured fragment.
func badges(labels []gh.Label, room int) string {
	var b strings.Builder
	for _, l := range labels {
		text := " " + l.Name + " "
		cost := ansi.StringWidth(text) + 1 // the space that separates badges
		if cost > room {
			break
		}
		b.WriteString(" " + theme.Badge(l.Color).Render(text))
		room -= cost
	}
	return b.String()
}

func stateMarker(it gh.WorkItem) string {
	if it.Ref.Kind == gh.ItemIssue {
		return theme.Dim().Render(icon.Issue())
	}
	return theme.Review(it.Review, it.IsDraft).Render(icon.Review(it.Review, it.IsDraft))
}

// cardMeta is the second line: the repository on the left, the checks bar and
// the elapsed time on the right. A column is about thirty columns wide, and
// fewer than twenty at eighty, so the right-hand pair is anchored and the
// repository takes whatever is left.
func cardMeta(it gh.WorkItem, w int, now time.Time) string {
	right := theme.Dim().Render(i18n.RelTime(now, it.UpdatedAt))
	if bar := checksBar(it.Checks); bar != "" {
		right = bar + " " + right
	}

	// One space keeps the repository off the bar even when both are full.
	room := w - len(gutter) - ansi.StringWidth(right) - 1
	if room <= 0 {
		return fit(gutter+right, w)
	}
	repo := theme.Dim().Render(clip(it.Ref.Repo, room))
	pad := w - len(gutter) - ansi.StringWidth(repo) - ansi.StringWidth(right)
	return gutter + repo + strings.Repeat(" ", max(pad, 0)) + right
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

// drawer names the selected card in full and spells out its checks, which the
// card itself only has room to draw as a bar.
func (m Model) drawer() []string {
	ref, ok := m.SelectedRef()
	if !ok {
		return nil
	}
	it := m.work[m.section()][m.row]

	lines := []string{
		strings.Repeat("─", m.width),
		clip(fmt.Sprintf("%s#%d %s", ref.Repo, ref.Number, it.Title), m.width),
	}
	// Issues have no checks at all, so they get no checks line either.
	if ref.Kind == gh.ItemIssue {
		return lines
	}

	summary := i18n.T("work.no_checks")
	if c := it.Checks; c.Total > 0 {
		summary = i18n.Tf("work.checks_summary", map[string]any{
			"Passed": c.Passed, "Total": c.Total, "Failed": c.Failed, "Running": c.Running,
		})
	}
	return append(lines, theme.Dim().Render(clip(summary, m.width)))
}

// visibleSections is the width degradation: too narrow for four columns and
// the board shows the current one alone, with h/l paging between them.
func (m Model) visibleSections() []gh.WorkSection {
	if m.width < singleColumnBelow {
		return []gh.WorkSection{m.section()}
	}
	return gh.WorkSections()
}

func (m Model) columnWidth(n int) int {
	return (m.width - columnGap*(n-1)) / n
}

// clip cuts s to w display columns. Japanese takes two columns per character,
// so the count is never a byte or a rune count.
func clip(s string, w int) string {
	return ansi.Truncate(s, w, "…")
}

// fit clips s and pads it out to exactly w display columns.
func fit(s string, w int) string {
	s = clip(s, w)
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
