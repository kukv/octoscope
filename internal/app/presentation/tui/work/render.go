package work

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/drawer"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

const (
	// columnGap is the vertical rule between two columns and the space either
	// side of it.
	columnGap = 3

	// twoColumnsBelow and singleColumnBelow are where the board gives up a
	// column. A card's head line — the marker, "owner/name" and the number —
	// wants about twenty columns, and four columns leave a card (W-9)/4-4
	// wide: eighteen at a hundred, twenty-three at a hundred and twenty.
	twoColumnsBelow   = 120
	singleColumnBelow = 80

	// footerHeight is the blank line and the key bar under the board.
	footerHeight = 2

	// gutter indents what is drawn outside a card — the heading, the spinner,
	// the empty-column note — by as much as a card's own border and padding,
	// so a column reads as one edge rather than two.
	gutter = "  "
)

// sectionTitleIDs maps a column to its heading in the catalog.
var sectionTitleIDs = map[domain.WorkSection]string{
	domain.SectionReviewRequested: "work.review_requested",
	domain.SectionYourPRs:         "work.your_prs",
	domain.SectionAssigned:        "work.assigned",
	domain.SectionMentioned:       "work.mentioned",
}

func (m Model) View() string {
	// Before the first WindowSizeMsg there is no width to lay anything out in,
	// and every budget below would go negative.
	if m.width <= 0 {
		return ""
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
		lines = append(lines, m.drawerLines()...)
	}
	lines = append(lines, "")
	if n := m.noticeLine(); n != "" {
		lines = append(lines, n)
	}
	return strings.Join(append(lines, m.keyBar()), "\n")
}

// noticeLine is what the board could not fetch, above the key bar. Several
// columns can fail at once and only one line is on offer, so it is the first
// in the order the columns are drawn in: the notice names no column, and any
// other choice would be one the user cannot follow. Which column failed is
// told by the column itself (see columnLines).
func (m Model) noticeLine() string {
	for _, s := range domain.WorkSections() {
		if m.notice[s] == "" {
			continue
		}
		text := i18n.T("notice.fetch_failed") + " · " + m.notice[s]
		return theme.Error().Render(layout.Notice(text, m.width))
	}
	return ""
}

// hasNotice is the question the height budget asks. It is not noticeLine() !=
// "": that reads the catalog and styles a line, and the budget is worked out
// for every frame.
func (m Model) hasNotice() bool {
	return slices.ContainsFunc(m.notice[:], func(s string) bool { return s != "" })
}

func (m Model) keyBar() string {
	return theme.Dim().Render(clip(i18n.T("footer.work"), m.width))
}

func (m Model) drawerShown() bool { return m.width >= drawer.MinColumns }

// drawerLines is the drawer for whatever the cursor is on. An empty column
// has nothing to show and still takes the same height, or the key bar would
// move as the cursor crossed into it.
func (m Model) drawerLines() []string {
	if _, ok := m.SelectedRef(); !ok {
		return drawer.Empty(m.width)
	}
	return drawer.Render(m.work[m.section()][m.row], m.width)
}

// titleLines is fixed rather than fitted to the title. visibleCards,
// cardWindow and the mouse hit-test all divide by the card height, so a card
// that shrank with a short title would put them out by however many short
// titles sat above the pointer.
const titleLines = 2

// cardHeight is how many lines one card occupies: the head line, the title,
// the meta line under it, and the box's two borders.
func cardHeight() int { return titleLines + 4 }

// boardHeight is what is left for the columns once everything drawn below
// them has been paid for. The whole screen has to fit: a board that grew with
// the longest column pushed the drawer and the key bar off the terminal.
func (m Model) boardHeight() int {
	if m.height <= 0 {
		return 0 // no height yet: draw every card and let the caller cope
	}
	h := m.height - m.boardTop() - footerHeight
	if m.drawerShown() {
		h -= drawer.Height
	}
	if m.hasNotice() {
		h--
	}
	return max(h, headingHeight+cardHeight())
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
// are divided by the vertical rules between them.
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
// allows, starting from the offset that keeps the cursor in view. A column
// still waiting on its own request shows a spinner in the space its cards
// will take, so the columns that have answered stay readable.
func (m Model) columnLines(s domain.WorkSection, w, height int) []string {
	items := m.work[s]
	lines := []string{m.heading(s, len(items), w)}
	if m.state[s] == colLoading {
		// The spinner carries its own colour, so it is not wrapped in a style
		// that would end at the spinner's own reset.
		return append(lines, layout.Fill(gutter+m.spin.View()+" "+i18n.T("common.loading"), w))
	}
	if len(items) == 0 {
		// The empty-column text would report an outage as good news. Only a
		// column that has never been answered is empty in the first place: a
		// failed refetch keeps the cards it had, which is the point.
		if m.state[s] == colFailed {
			return append(lines, theme.Error().Render(layout.Fill(gutter+i18n.T("notice.fetch_failed"), w)))
		}
		return append(lines, theme.Dim().Render(layout.Fill(gutter+i18n.T("work.empty_column"), w)))
	}

	first, last := 0, len(items)
	if height > 0 {
		first = m.cardWindow(s, height)
		last = min(first+m.visibleCards(height), len(items))
	}
	for i := first; i < last; i++ {
		lines = append(lines, m.card(items[i], m.fetchedAt[s], w, s == m.section() && i == m.row)...)
	}
	return lines
}

// heading names the column and counts what is in it. The count is the point
// of the board: the length of a column is how much has piled up, and the
// number says so even when the column is scrolled.
func (m Model) heading(s domain.WorkSection, n, w int) string {
	name := i18n.T(sectionTitleIDs[s])
	count := ""
	if n > 0 {
		count = strconv.Itoa(n)
	}
	room := w - len(gutter) - ansi.StringWidth(count) - 1
	name = theme.Heading().Render(clip(name, max(room, 0)))
	pad := w - len(gutter) - ansi.StringWidth(name) - ansi.StringWidth(count)
	return gutter + name + strings.Repeat(" ", max(pad, 0)) +
		theme.Count(s == domain.SectionReviewRequested && n > 0).Render(count)
}

// visibleCards is how many whole cards fit under a heading.
func (m Model) visibleCards(height int) int {
	return max((height-headingHeight)/cardHeight(), 1)
}

// cardWindow is the first card a column draws. Only the column the cursor is
// in scrolls; the others start at the top, because their own position is not
// something the user is steering.
func (m Model) cardWindow(s domain.WorkSection, height int) int {
	if s != m.section() {
		return 0
	}
	visible := m.visibleCards(height)
	if m.row < visible {
		return 0
	}
	return m.row - visible + 1
}

// card draws one card in a box of its own: where it lives on the first line,
// its title on the next two, how it is doing on the last. The selection is
// the box's colour and background.
//
// The body's lines are filled one at a time before the box is drawn around
// them. lipgloss fills its own padding, but the background it is given ends
// at the first reset inside the text, and every line here has a coloured
// marker or a dimmed repository in it.
func (m Model) card(it domain.WorkItem, at time.Time, w int, selected bool) []string {
	// The box's own border and padding come out of the width lipgloss is
	// given, so the text is clipped to what is left before it is handed over.
	inner := w - 4
	body := append([]string{cardHead(it, inner)}, cardTitle(it, inner, selected)...)
	body = append(body, m.cardMeta(it, at, inner))
	if selected {
		for i, line := range body {
			body[i] = theme.SelectedLine(line)
		}
	}
	return strings.Split(theme.Card(selected).Width(w).Render(strings.Join(body, "\n")), "\n")
}

// cardHead is the first line: what state the item is in, which repository it
// came from and which number it is there. The repository is clipped before
// the number is — the number is what identifies the card, and half a number
// identifies nothing.
//
// The pieces are styled one at a time rather than as a whole line: a style
// applied over a coloured marker would end at that marker's own reset.
func cardHead(it domain.WorkItem, w int) string {
	head := stateMarker(it) + " "
	number := fmt.Sprintf(" #%d", it.Ref.Number)
	repo := clip(it.Ref.Repo, max(w-ansi.StringWidth(head)-ansi.StringWidth(number), 0))
	return head + theme.Dim().Render(repo) + number
}

// cardTitle is the title alone, over titleLines lines.
func cardTitle(it domain.WorkItem, w int, selected bool) []string {
	first, second := wrapTitle(it.Title, max(w, 0))
	if selected {
		first, second = theme.Cursor().Render(first), theme.Cursor().Render(second)
	}
	return []string{first, second}
}

// wrapTitle folds a title over two lines of w columns each.
//
// The second line is cut from the title itself rather than joined back up out
// of the lines ansi.Wrap returned. A Japanese title has no spaces to break on
// and comes back broken by column, and joining those pieces would put spaces
// in the title that the author never wrote.
func wrapTitle(title string, w int) (string, string) {
	head := strings.Split(ansi.Wrap(title, max(w, 1), ""), "\n")[0]
	return head, clip(strings.TrimSpace(strings.TrimPrefix(title, head)), w)
}

// cardMeta is the last line: how the item's checks are doing, how long it has
// sat there, and what it is labelled. Where it lives is on the head line.
func (m Model) cardMeta(it domain.WorkItem, at time.Time, w int) string {
	var parts []string
	if bar := checksBar(it.Checks); bar != "" {
		parts = append(parts, bar)
	} else if word := reviewWord(it); word != "" {
		parts = append(parts, word)
	}
	parts = append(parts, theme.Dim().Render(i18n.RelTime(at, it.UpdatedAt)))

	// Labels are offered whatever the rest of the line has not already spent,
	// so a badge is either drawn whole or left out. Measuring against the
	// whole width would let the clip below cut one in half, which reads as a
	// coloured smear rather than a label.
	spent := ansi.StringWidth(strings.Join(parts, " ")) + 1
	if b := theme.Badges(it.Labels, w-spent); b != "" {
		parts = append(parts, strings.TrimSpace(b))
	}
	return clip(strings.Join(parts, " "), w)
}

// reviewWord is what a pull request with no checks says instead of a bar.
// Issues say nothing: they have neither checks nor a review.
func reviewWord(it domain.WorkItem) string {
	if it.Ref.Kind != domain.ItemPR {
		return ""
	}
	style := theme.Review(it.Review, it.IsDraft)
	switch {
	case it.IsDraft:
		return style.Render(i18n.T("work.draft"))
	case it.Review == domain.ReviewApproved:
		return style.Render(i18n.T("review.approved"))
	case it.Review == domain.ReviewChangesRequested:
		return style.Render(i18n.T("review.changes_requested"))
	default:
		return ""
	}
}

func stateMarker(it domain.WorkItem) string {
	if it.Ref.Kind == domain.ItemIssue {
		return theme.Issue().Render(icon.Issue())
	}
	return theme.Review(it.Review, it.IsDraft).Render(icon.Review(it.Review, it.IsDraft))
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

// columnsFor is the width degradation: how many columns the board puts side
// by side. The four sections divide by four, two and one, so each tier pages
// by whole pages; three columns would leave a page with one column in it.
func (m Model) columnsFor() int {
	switch {
	case m.width < singleColumnBelow:
		return 1
	case m.width < twoColumnsBelow:
		return 2
	default:
		return m.columns()
	}
}

// visibleSections is the page of columns on screen: the one the cursor's
// column falls in. h/l move the cursor, and the page follows it.
func (m Model) visibleSections() []domain.WorkSection {
	n := m.columnsFor()
	all := domain.WorkSections()
	page := m.col / n
	return all[page*n : min((page+1)*n, len(all))]
}

func (m Model) columnWidth(n int) int {
	return (m.width - columnGap*(n-1)) / n
}

// clip cuts s to w display columns. Japanese takes two columns per character,
// so the count is never a byte or a rune count.
func clip(s string, w int) string {
	return ansi.Truncate(s, w, "…")
}
