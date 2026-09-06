package work

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

const (
	// columnGap is the vertical rule between two columns and the space either
	// side of it.
	columnGap = 3

	// drawerMinColumns is also cardBoxMinColumns: under a hundred columns a
	// column is about seventeen wide, and a card's own box would eat two of
	// them and leave the title with almost nothing. Both the drawer and the
	// boxes go at the same width, so there is one number to remember
	// (spec 4.6).
	drawerMinColumns  = 100
	singleColumnBelow = 60

	// footerHeight is the blank line and the key bar under the board.
	footerHeight = 2

	// drawerHeight is fixed: the drawer sits below a board whose length
	// depends on whichever column holds the most cards, and a drawer that
	// changed height would move the key bar under the user's eyes.
	drawerHeight = 6

	// gutter is the column the cursor marker lives in on an unboxed card.
	// Unselected cards keep it blank rather than closing it up, so card text
	// does not jump sideways as the cursor moves.
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
	// and every budget below would go negative.
	if m.width <= 0 {
		return ""
	}
	if m.loading {
		return clip(m.spin.View()+" "+i18n.T("common.loading"), m.width)
	}

	var lines []string
	if m.boardTop() > 0 {
		position := i18n.Tf("work.column_position", map[string]any{
			"Index": m.col + 1,
			"Total": m.columns(),
		})
		lines = append(lines, theme.Dim().Render(clip(position, m.width)), "")
	}
	lines = append(lines, m.board(m.boardHeight())...)
	if m.drawerShown() {
		lines = append(lines, m.drawer()...)
	}
	return strings.Join(append(lines, "", m.keyBar()), "\n")
}

func (m Model) keyBar() string {
	return theme.Dim().Render(clip(i18n.T("footer.work"), m.width))
}

func (m Model) drawerShown() bool { return m.width >= drawerMinColumns }

// boxed reports whether cards are drawn in their own box. See
// drawerMinColumns for why the two share a threshold.
func (m Model) boxed() bool { return m.width >= drawerMinColumns }

// cardHeight is how many lines one card occupies: a box adds its two borders
// to the title and meta lines inside it.
func (m Model) cardHeight() int {
	if m.boxed() {
		return 4
	}
	return 2
}

// boardHeight is what is left for the columns once everything drawn below
// them has been paid for. The whole screen has to fit: a board that grew with
// the longest column pushed the drawer and the key bar off the terminal.
func (m Model) boardHeight() int {
	if m.height <= 0 {
		return 0 // no height yet: draw every card and let the caller cope
	}
	h := m.height - m.boardTop() - footerHeight
	if m.drawerShown() {
		h -= drawerHeight
	}
	return max(h, headingHeight+m.cardHeight())
}

// boardTop is the line the board starts on. Paging between single columns
// puts a "column 2/4" note and a blank line above it; four columns start at
// the top. The mouse hit-test reads the same function View does, so the two
// cannot drift apart.
func (m Model) boardTop() int {
	if len(m.visibleSections()) < m.columns() {
		return 2
	}
	return 0
}

// headingHeight is the column heading. There is no rule under it: the columns
// are divided by the vertical rules between them (spec 4.1).
const headingHeight = 1

// board lays the columns side by side, separated by a vertical rule. Every
// cell is padded to the column width first, so the columns stay aligned
// however many cards each one holds.
//
// A height of zero means "no budget yet": every card is drawn.
func (m Model) board(height int) []string {
	sections := m.visibleSections()
	w := m.columnWidth(len(sections))
	columns := make([][]string, len(sections))
	filled := 0
	for i, s := range sections {
		columns[i] = m.columnLines(s, w, height)
		filled = max(filled, len(columns[i]))
	}

	rule := theme.Rule().Render("│")
	blank := strings.Repeat(" ", w)
	lines := make([]string, max(height, filled))
	for row := range lines {
		// Below the longest column there is nothing to divide. Running the
		// rules to the bottom of the budget would draw a frame around empty
		// space and hide what the board is for: how far each column reaches
		// is how much has piled up in it.
		if row >= filled {
			continue
		}
		cells := make([]string, len(columns))
		for i, column := range columns {
			cells[i] = blank
			if row < len(column) {
				cells[i] = column[row]
			}
		}
		lines[row] = strings.TrimRight(strings.Join(cells, " "+rule+" "), " ")
	}
	return lines
}

// columnLines draws one column: its heading, then as many cards as the height
// allows, starting from the offset that keeps the cursor in view.
func (m Model) columnLines(s gh.WorkSection, w, height int) []string {
	items := m.work[s]
	lines := []string{m.heading(s, len(items), w)}
	if len(items) == 0 {
		return append(lines, theme.Dim().Render(fit(gutter+i18n.T("work.empty_column"), w)))
	}

	first, last := 0, len(items)
	if height > 0 {
		first = m.cardWindow(s, height)
		last = min(first+m.visibleCards(height), len(items))
	}
	for i := first; i < last; i++ {
		lines = append(lines, m.card(items[i], w, s == m.section() && i == m.row)...)
	}
	return lines
}

// heading names the column and counts what is in it. The count is the point
// of the board: the length of a column is how much has piled up, and the
// number says so even when the column is scrolled.
func (m Model) heading(s gh.WorkSection, n, w int) string {
	name := i18n.T(sectionTitleIDs[s])
	count := ""
	if n > 0 {
		count = strconv.Itoa(n)
	}
	room := w - len(gutter) - ansi.StringWidth(count) - 1
	name = theme.Heading().Render(clip(name, max(room, 0)))
	pad := w - len(gutter) - ansi.StringWidth(name) - ansi.StringWidth(count)
	return gutter + name + strings.Repeat(" ", max(pad, 0)) +
		theme.Count(s == gh.SectionReviewRequested && n > 0).Render(count)
}

// visibleCards is how many whole cards fit under a heading.
func (m Model) visibleCards(height int) int {
	return max((height-headingHeight)/m.cardHeight(), 1)
}

// cardWindow is the first card a column draws. Only the column the cursor is
// in scrolls; the others start at the top, because their own position is not
// something the user is steering.
func (m Model) cardWindow(s gh.WorkSection, height int) int {
	if s != m.section() {
		return 0
	}
	visible := m.visibleCards(height)
	if m.row < visible {
		return 0
	}
	return m.row - visible + 1
}

// card draws one card: what it is on the first line, where it lives and how
// it is doing on the second (spec 4.1). Wide enough, each card gets a box of
// its own and the selection is the box's colour; narrow, the box is dropped
// and the cursor gutter marks the selection instead.
func (m Model) card(it gh.WorkItem, w int, selected bool) []string {
	if !m.boxed() {
		return []string{
			fit(m.cardTitle(it, w-len(gutter), selected, gutter), w),
			fit(gutter+m.cardMeta(it, w-len(gutter)), w),
		}
	}
	// The box's own border and padding come out of the width lipgloss is
	// given, so the text is clipped to what is left before it is handed over.
	inner := w - 4
	body := m.cardTitle(it, inner, selected, "") + "\n" + m.cardMeta(it, inner)
	return strings.Split(theme.Card(selected).Width(w).Render(body), "\n")
}

// cardTitle is the state marker, the number and the title. The pieces are
// styled one at a time rather than as a whole line: a style applied over a
// coloured marker would end at that marker's own reset.
func (m Model) cardTitle(it gh.WorkItem, w int, selected bool, marker string) string {
	if selected && marker != "" {
		marker = theme.Cursor().Render("▸ ")
	}
	head := marker + stateMarker(it) + " " + fmt.Sprintf("#%d ", it.Ref.Number)
	title := clip(it.Title, max(w-ansi.StringWidth(head), 0))
	if selected {
		title = theme.Cursor().Render(title)
	}
	return head + title
}

// cardMeta is the second line: where the item lives, how its checks are
// doing, and how long it has sat there. The repository is named without its
// owner — a column is too narrow for "owner/name", and the drawer gives the
// full reference.
func (m Model) cardMeta(it gh.WorkItem, w int) string {
	parts := []string{theme.Dim().Render(shortRepo(it.Ref.Repo))}
	if bar := checksBar(it.Checks); bar != "" {
		parts = append(parts, bar)
	} else if word := reviewWord(it); word != "" {
		parts = append(parts, word)
	}
	age := theme.Dim().Render(i18n.RelTime(m.fetchedAt, it.UpdatedAt))

	// Labels are offered whatever the rest of the line has not already spent,
	// so a badge is either drawn whole or left out. Measuring against the
	// whole width would let the clip below cut one in half, which reads as a
	// coloured smear rather than a label.
	spent := ansi.StringWidth(strings.Join(parts, " ")) + 1 + ansi.StringWidth(age) + 1
	if b := badges(it.Labels, w-spent); b != "" {
		parts = append(parts, strings.TrimSpace(b))
	}
	return clip(strings.Join(append(parts, age), " "), w)
}

// reviewWord is what a pull request with no checks says instead of a bar.
// Issues say nothing: they have neither checks nor a review.
func reviewWord(it gh.WorkItem) string {
	if it.Ref.Kind != gh.ItemPR {
		return ""
	}
	style := theme.Review(it.Review, it.IsDraft)
	switch {
	case it.IsDraft:
		return style.Render(i18n.T("work.draft"))
	case it.Review == gh.ReviewApproved:
		return style.Render(i18n.T("review.approved"))
	case it.Review == gh.ReviewChangesRequested:
		return style.Render(i18n.T("review.changes_requested"))
	default:
		return ""
	}
}

// shortRepo drops the owner from "owner/name".
func shortRepo(repo string) string {
	if _, name, ok := strings.Cut(repo, "/"); ok {
		return name
	}
	return repo
}

// badges draws the labels that fit in room columns, in the colours GitHub
// gave them (spec 4.5). A label that would be cut in half is left out
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

// checksBar colours the two halves of the bar apart: what has passed takes the
// colour of the roll-up, what has not stays muted.
func checksBar(c gh.Checks) string {
	done, rest := icon.ChecksBar(c)
	if done == "" && rest == "" {
		return ""
	}
	return theme.Check(c.State).Render(done) + theme.Dim().Render(rest)
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
