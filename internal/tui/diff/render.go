package diff

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

	// composerRows is the line-comment composer's fixed textarea height.
	composerRows = 3

	// minWidthForSidebar is where the file list stops earning its columns.
	// Below it the body would be 46 columns, which is 23 Japanese
	// characters. It matches the width at which the Work board drops its
	// card borders (spec 4.6).
	minWidthForSidebar = 100
)

// showSidebar reports whether the file list is drawn at all. Task 11's hit
// testing reads this too, so the threshold lives in one place.
func (m Model) showSidebar() bool { return m.width >= minWidthForSidebar }

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
	if m.reviewErr != nil {
		lines = append(lines, m.reviewErrLine())
	}
	if m.composing || m.posting {
		lines = append(lines, m.composerLines()...)
	}
	if m.submitting {
		lines = append(lines, m.submitLines()...)
	}
	if m.discarding {
		lines = append(lines, m.discardLines()...)
	}
	return strings.Join(append(lines, m.keyBar()), "\n")
}

func (m Model) keyBar() string {
	switch {
	case m.composing || m.posting:
		return theme.Dim().Render(clip(i18n.T("footer.diff_comment"), m.width))
	case m.submitting:
		return theme.Dim().Render(clip(i18n.T("footer.submit"), m.width))
	case m.discarding:
		return theme.Dim().Render(clip(i18n.T("footer.discard"), m.width))
	default:
		return theme.Dim().Render(layout.FitKeyBar(diffHints(), m.width))
	}
}

// diffHints is the diff view's key bar, most important hint first. esc is
// first because layout.FitKeyBar never drops it: it is the only way out of the
// view, and every other hint is either repeated elsewhere on screen or
// discoverable by trying the obvious key. v comes right after comment --
// it is the point of this phase -- and X sits last: it is rare and
// destructive, and the first thing to go when the terminal is narrow.
func diffHints() []string {
	return []string{
		i18n.T("footer.diff.esc"),
		i18n.T("footer.diff.comment"),
		i18n.T("footer.diff.review"),
		i18n.T("footer.diff.line"),
		i18n.T("footer.diff.file"),
		i18n.T("footer.diff.hunk"),
		i18n.T("footer.diff.open"),
		i18n.T("footer.diff.pane"),
		i18n.T("footer.diff.refresh"),
		i18n.T("footer.diff.discard"),
	}
}

// submitLines draws the review popup over the diff: a blank separator, the
// popup's own box, and a failed submission's error underneath it. The popup
// keeps no error text of its own, so what the user typed and chose is still
// there for a retry (.claude/rules/errors.md).
func (m Model) submitLines() []string {
	lines := append([]string{""}, strings.Split(m.submit.View(), "\n")...)
	if m.submitErr != "" {
		lines = append(lines, clip(theme.Error().Render(i18n.T("common.error_prefix"))+singleLine(m.submitErr), m.width))
	}
	return lines
}

// submitHeight is what submitLines takes, out of the pane's height budget the
// same way composerHeight is, so the popup never pushes the key bar off the
// bottom.
func (m Model) submitHeight() int {
	if !m.submitting {
		return 0
	}
	h := 1 + len(strings.Split(m.submit.View(), "\n"))
	if m.submitErr != "" {
		h++
	}
	return h
}

// discardLines draws the discard confirmation: the question, and either the
// spinner while DiscardReview is running or a failure underneath it.
func (m Model) discardLines() []string {
	lines := []string{"", clip(theme.Error().Render(i18n.T("submit.discard_confirm")), m.width)}
	switch {
	case m.discardErr != "":
		lines = append(lines, clip(theme.Error().Render(i18n.T("common.error_prefix"))+singleLine(m.discardErr), m.width))
	case m.discardWorking:
		lines = append(lines, clip(m.spin.View()+" "+i18n.T("confirm.working"), m.width))
	}
	return lines
}

// discardHeight is what discardLines takes, out of the pane's height budget
// the same way submitHeight is.
func (m Model) discardHeight() int {
	if !m.discarding {
		return 0
	}
	h := 2
	if m.discardErr != "" || m.discardWorking {
		h++
	}
	return h
}

// composerLines draws the line-comment composer: a blank separator, the
// textarea, and at most one more line -- a failed post's error, or the
// spinner while the comment is in flight. composerHeight has to agree with
// how many lines this returns, or the pane's height budget goes stale and
// the key bar gets pushed off the bottom.
func (m Model) composerLines() []string {
	lines := append([]string{""}, strings.Split(m.textarea.View(), "\n")...)
	switch {
	case m.postErr != "":
		lines = append(lines, clip(theme.Error().Render(i18n.T("common.error_prefix"))+singleLine(m.postErr), m.width))
	case m.posting:
		lines = append(lines, clip(m.spin.View()+" "+i18n.T("diff.posting"), m.width))
	}
	return lines
}

// composerHeight is what composerLines takes, out of the pane's height
// budget the same way reviewErrHeight is, so the composer never pushes the
// key bar off the bottom.
func (m Model) composerHeight() int {
	if !m.composing && !m.posting {
		return 0
	}
	h := 1 + composerRows
	if m.postErr != "" || m.posting {
		h++
	}
	return h
}

// reviewErrLine reports a review-context fetch that failed. The diff itself
// may still be readable, so this is one line above the key bar rather than
// the parent's whole-screen error view (.claude/rules/errors.md). GitHub's
// own wording carries the most information, so only the prefix is
// translated.
func (m Model) reviewErrLine() string {
	text := theme.Error().Render(i18n.T("common.error_prefix")) + singleLine(m.reviewErr.Error())
	return clip(text, m.width)
}

// reviewErrHeight is the extra line reviewErrLine takes, taken out of the
// pane's height budget the same way the key bar is, so a failure never pushes
// the key bar off the bottom.
func (m Model) reviewErrHeight() int {
	if m.reviewErr != nil {
		return 1
	}
	return 0
}

// header draws the two lines the diff view can fill in: the pull request's
// own name and title, and how big the change is. The title and the branches
// arrive with the review context, which may still be loading when the diff
// itself has landed, so both are blank until then rather than the view
// waiting on the slower of the two fetches.
func (m Model) header() []string {
	first := fmt.Sprintf("%s #%d", m.ref.Repo, m.ref.Number)
	if m.review.Title != "" {
		first += " " + m.review.Title
	}
	return []string{
		clip(theme.Title().Render(first), m.width),
		clip(m.sizeLine(), m.width),
		m.filesHeadingLine(),
	}
}

// sizeLine is the branches, how big the diff is, and how many unsubmitted
// comments are waiting to go out.
func (m Model) sizeLine() string {
	adds, dels := 0, 0
	for _, f := range m.files {
		adds += f.Additions
		dels += f.Deletions
	}
	var parts []string
	if m.review.Head != "" || m.review.Base != "" {
		parts = append(parts, m.review.Head+" → "+m.review.Base)
	}
	size := i18n.Tn("diff.file_count", len(m.files))
	if adds > 0 || dels > 0 {
		size += " " + theme.Added().Render("+"+strconv.Itoa(adds)) +
			" " + theme.Removed().Render("−"+strconv.Itoa(dels))
	}
	parts = append(parts, size)
	if n := m.pendingCount(); n > 0 {
		parts = append(parts, i18n.Tn("diff.pending_note", n))
	}
	// The folded sidebar takes the file list with it, so the file being
	// read is named here instead: a path is GitHub's content, not
	// translated (.claude/rules/tui.md).
	if !m.showSidebar() {
		if path := m.currentPath(); path != "" {
			parts = append(parts, path)
		}
	}
	return theme.Dim().Render(strings.Join(parts, " · "))
}

// pendingCount is how many comments across all threads have not been
// submitted yet.
func (m Model) pendingCount() int { return m.review.PendingCount() }

// filesHeadingLine is the rule that separates the header from the two
// panes, with the "Files" heading over the sidebar column. Folded, there is
// no sidebar column to head, so it is a plain rule the full width.
func (m Model) filesHeadingLine() string {
	if !m.showSidebar() {
		return theme.Rule().Render(strings.Repeat("─", m.width))
	}
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
	return max(m.height-headerHeight-keyBarHeight-m.reviewErrHeight()-m.composerHeight()-m.submitHeight()-m.discardHeight(), 1)
}

// body lays the sidebar and the diff pane side by side, the way the Repos
// tab lays its two panes out. Folded, there is no sidebar column at all, and
// the diff pane takes the full width.
func (m Model) body() []string {
	h := m.paneHeight()
	pane := m.diffLines()
	if !m.showSidebar() {
		lines := make([]string, h)
		for i := range lines {
			if i < len(pane) {
				lines[i] = pane[i]
			}
		}
		return lines
	}

	sidebar := m.sidebarLines()
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
// It starts at m.fileTop, which followSidebar keeps in step with the
// selected file, the same way m.top keeps the diff pane's cursor on screen.
func (m Model) sidebarLines() []string {
	if len(m.files) == 0 {
		return nil
	}
	lines := make([]string, 0, (len(m.files)-m.fileTop)*2)
	for i := m.fileTop; i < len(m.files); i++ {
		f := m.files[i]
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
	width := m.width
	if m.showSidebar() {
		width = max(m.width-sidebarWidth-1, 0)
	}
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
	case rowThread:
		return m.threadLine(r, width)
	case rowCollapsed:
		return theme.Dim().Render(clip(icon.Collapsed()+" "+r.text, width))
	default:
		return m.diffTextLine(r.line, width)
	}
}

// threadLine draws one comment of an open review thread: a bar marking it
// apart from the code, the author, and the body, coloured by whether it has
// been sent yet.
func (m Model) threadLine(r row, width int) string {
	return theme.Thread(r.comment.Pending).Render(clip(m.threadText(r), width))
}

// plainText is what a row reads as with no styling at all, for the cursor
// row.
func (m Model) plainText(r row) string {
	switch r.kind {
	case rowLine:
		old, marker, num := lineNumbers(r.line)
		return fmt.Sprintf("%*s %*s%s %s", m.lineNumberWidth(), old, m.lineNumberWidth(), num, marker, r.line.Text)
	case rowThread:
		return m.threadText(r)
	case rowCollapsed:
		return icon.Collapsed() + " " + r.text
	default:
		return r.text
	}
}

// threadText is what threadLine draws, without its colour.
func (m Model) threadText(r row) string {
	body := r.comment.Author.Login + " · " + singleLine(r.comment.Body)
	if r.comment.Pending {
		body += " (" + i18n.T("diff.unsent") + ")"
	}
	return icon.CommentBar() + " " + body
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

// singleLine folds a GitHub body or error text onto one row. A review
// comment is markdown and routinely has newlines in it, and View joins one
// string per row: a body carrying its own newline would draw as extra visual
// rows and shift every row under it down by one, the same failure as a tab
// left unexpanded. Folding every run of whitespace to a single space keeps as
// much of the text visible as fits, rather than showing only its first line.
func singleLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// clip cuts s to w display columns. Japanese takes two columns per
// character, so the count is never a byte or a rune count.
func clip(s string, w int) string { return ansi.Truncate(s, w, "…") }

// fit clips s and pads it out to exactly w display columns.
func fit(s string, w int) string {
	s = clip(s, w)
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
