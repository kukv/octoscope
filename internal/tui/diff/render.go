package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/theme"
)

const (
	// sidebarWidth is the file list's fixed width. Task 10's width
	// degradation reads this same constant rather than a second copy of it.
	sidebarWidth = 22

	// gutterWidth is the floor: the old line number (4), a space, the new
	// line number (4), the +/- marker (1) and the space that separates it
	// from the text (1). It is a floor, not a fixed size: a file whose line
	// numbers run to five digits or more needs a wider gutter, computed by
	// Model.gutter, which both the drawing and Task 11's hit-test read.
	gutterWidth = 11

	// headerHeight is the two header lines plus the rule and "Files" heading
	// under them. It is a constant, not a computed length, so the mouse
	// hit-test (Task 11) can read it without laying the screen out a second
	// time.
	headerHeight = 3

	// keyBarHeight is the single line at the bottom of the screen.
	keyBarHeight = 1
)

func (m Model) View() string {
	// Before the first WindowSizeMsg there is no width to lay anything out
	// in, and every budget below would go negative.
	if m.width <= 0 {
		return ""
	}
	if m.loading {
		return clip(m.spin.View()+" "+i18n.T("diff.loading"), m.width)
	}

	lines := append(m.header(), m.body()...)
	return strings.Join(append(lines, m.keyBar()), "\n")
}

func (m Model) keyBar() string {
	return theme.Dim().Render(clip(i18n.T("footer.diff"), m.width))
}

// header draws the two lines the diff view can fill in on its own: the
// pull request's own name, and how big the change is. The title, the
// branches and the thread count arrive with the review context in Task 7.
func (m Model) header() []string {
	first := fmt.Sprintf("%s #%d", m.ref.Repo, m.ref.Number)
	return []string{
		clip(theme.Title().Render(first), m.width),
		clip(m.sizeLine(), m.width),
		m.filesHeadingLine(),
	}
}

// sizeLine counts the diff itself: how many files, and how much they add and
// remove.
func (m Model) sizeLine() string {
	adds, dels := 0, 0
	for _, f := range m.files {
		adds += f.Additions
		dels += f.Deletions
	}
	parts := []string{i18n.Tn("diff.file_count", len(m.files))}
	if adds > 0 || dels > 0 {
		parts = append(parts, theme.Added().Render("+"+strconv.Itoa(adds))+
			" "+theme.Removed().Render("−"+strconv.Itoa(dels)))
	}
	return theme.Dim().Render(strings.Join(parts, " "))
}

// filesHeadingLine is the rule that separates the header from the two
// panes, with the "Files" heading over the sidebar column.
func (m Model) filesHeadingLine() string {
	left := fit(theme.Heading().Render(i18n.T("diff.files")), sidebarWidth)
	div := theme.Rule().Render("│")
	rest := max(m.width-sidebarWidth-ansi.StringWidth(div), 0)
	return left + div + theme.Rule().Render(strings.Repeat("─", rest))
}

// paneHeight is what is left for the sidebar and the diff pane once the
// header and the key bar have been paid for. The whole screen has to fit: a
// long file must never push the key bar off the bottom.
func (m Model) paneHeight() int {
	if m.height <= 0 {
		return 0
	}
	return max(m.height-headerHeight-keyBarHeight, 1)
}

// body lays the sidebar and the diff pane side by side, the way the Repos
// tab lays its two panes out.
func (m Model) body() []string {
	h := m.paneHeight()
	sidebar := m.sidebarLines()
	pane := m.diffLines()
	div := theme.Rule().Render("│")

	lines := make([]string, h)
	for i := range lines {
		left := ""
		if i < len(sidebar) {
			left = sidebar[i]
		}
		right := ""
		if i < len(pane) {
			right = pane[i]
		}
		lines[i] = fit(left, sidebarWidth) + div + right
	}
	return lines
}

// sidebarLines draws the file list: a path per file, truncated from the
// right when it does not fit, and the size of that file's change under it.
func (m Model) sidebarLines() []string {
	if len(m.files) == 0 {
		return nil
	}
	lines := make([]string, 0, len(m.files)*2)
	for i, f := range m.files {
		path := clip(f.Path, sidebarWidth)
		plainSize := fmt.Sprintf("+%d −%d", f.Additions, f.Deletions)
		selected := i == m.file && m.sidebar
		if selected {
			lines = append(lines,
				theme.Selected().Render(fit(path, sidebarWidth)),
				theme.Selected().Render(fit(plainSize, sidebarWidth)))
			continue
		}
		size := theme.Added().Render("+"+strconv.Itoa(f.Additions)) +
			" " + theme.Removed().Render("−"+strconv.Itoa(f.Deletions))
		lines = append(lines, path, size)
	}
	return lines
}

// diffLines draws the visible slice of the current file's rows, from top for
// paneHeight of them. Only the pane the cursor is in scrolls (follow), so
// this always starts at m.top.
func (m Model) diffLines() []string {
	width := max(m.width-sidebarWidth-1, 0)
	end := min(m.top+m.paneHeight(), len(m.rows))
	lines := make([]string, 0, end-m.top)
	for i := m.top; i < end; i++ {
		lines = append(lines, m.diffLine(m.rows[i], i == m.row && !m.sidebar, width))
	}
	return lines
}

// diffLine draws one row of the diff pane. A selected row is drawn with no
// colour of its own at all, then wrapped whole in theme.Selected(): every
// coloured span lipgloss or chroma renders ends in its own reset, so a
// background wrapped around one goes patchy from the first reset on. A
// plain, fully legible selection beats syntax colour on the one line it
// would otherwise break.
func (m Model) diffLine(r row, selected bool, width int) string {
	if selected {
		return theme.Selected().Render(fit(m.plainText(r), width))
	}
	switch r.kind {
	case rowHunkHeader:
		return theme.HunkHeader().Render(clip(r.text, width))
	case rowNote:
		return theme.Dim().Render(clip(r.text, width))
	default:
		return m.diffTextLine(r.line, width)
	}
}

// plainText is what a row reads as with no styling at all, for the cursor
// row.
func (m Model) plainText(r row) string {
	if r.kind != rowLine {
		return r.text
	}
	old, marker, num := lineNumbers(r.line)
	return fmt.Sprintf("%*s %*s%s %s", m.lineNumberWidth(), old, m.lineNumberWidth(), num, marker, r.line.Text)
}

// diffTextLine draws the gutter (two line numbers and the +/- marker) and
// the line's own text, syntax-highlighted.
func (m Model) diffTextLine(l gh.DiffLine, width int) string {
	old, _, num := lineNumbers(l)
	fw := m.lineNumberWidth()
	body := theme.Highlight(m.currentPath(), clip(l.Text, max(width-m.gutter(), 0)))
	gutter := theme.LineNumber().Render(fmt.Sprintf("%*s %*s", fw, old, fw, num)) +
		markerStyle(l.Kind) + " "
	return gutter + body
}

// gutter is the columns the line numbers and marker occupy: two
// lineNumberWidth fields, the space between them, the marker and the space
// that separates it from the text. The drawing and Task 11's hit-test both
// read this method, so neither can drift from the other (spec 4.0).
func (m Model) gutter() int { return 2*m.lineNumberWidth() + 3 }

// lineNumberWidth is how many columns the widest line number in the file
// currently shown needs, floored at gutterWidth's four digits. The format
// that draws a line number pads to a minimum rather than truncating, so a
// file whose numbers run past four digits must widen the gutter, or the row
// it draws runs past the budget the rest of the layout assumes.
func (m Model) lineNumberWidth() int {
	w := (gutterWidth - 3) / 2
	for _, r := range m.rows {
		if r.kind != rowLine {
			continue
		}
		w = max(w, len(strconv.Itoa(r.line.OldLine)), len(strconv.Itoa(r.line.NewLine)))
	}
	return w
}

// currentPath is the file the cursor is in, used to pick the syntax
// highlighter's lexer.
func (m Model) currentPath() string {
	if len(m.files) == 0 {
		return ""
	}
	return m.files[m.file].Path
}

// lineNumbers is a line's old and new numbers, blank on the side the line
// does not exist on, and the plain, uncoloured +/- marker between them.
func lineNumbers(l gh.DiffLine) (old, marker, num string) {
	if l.OldLine > 0 {
		old = strconv.Itoa(l.OldLine)
	}
	if l.NewLine > 0 {
		num = strconv.Itoa(l.NewLine)
	}
	switch l.Kind {
	case gh.LineAdded:
		marker = "+"
	case gh.LineRemoved:
		marker = "-"
	default:
		marker = " "
	}
	return old, marker, num
}

// markerStyle colours the +/- marker for a line that is not the cursor row.
func markerStyle(k gh.DiffLineKind) string {
	switch k {
	case gh.LineAdded:
		return theme.DiffAdded().Render("+")
	case gh.LineRemoved:
		return theme.DiffRemoved().Render("-")
	default:
		return " "
	}
}

// tabWidth is how many columns expandTabs turns a tab into. It matches
// lipgloss's own default, so a row that goes through Style.Render (the
// cursor row) and one that does not (chroma's formatter, which leaves a
// tab as a literal byte) measure the same either way.
const tabWidth = 4

// expandTabs replaces a literal tab with spaces. A raw tab has no fixed
// display width — a terminal advances it to the next tab stop, chroma's
// formatter counts it as zero, lipgloss's Style.Render silently expands it
// to four spaces — so every diff line is expanded once, before anything
// measures or truncates it, rather than trusting any of those three to
// agree.
func expandTabs(s string) string { return strings.ReplaceAll(s, "\t", strings.Repeat(" ", tabWidth)) }

// clip cuts s to w display columns. Japanese takes two columns per
// character, so the count is never a byte or a rune count.
func clip(s string, w int) string { return ansi.Truncate(s, w, "…") }

// fit clips s and pads it out to exactly w display columns.
func fit(s string, w int) string {
	s = clip(s, w)
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
