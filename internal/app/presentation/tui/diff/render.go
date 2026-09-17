package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

const (
	// sidebarWidth is the file list's fixed width. The drawing and the mouse
	// hit-test read this same constant rather than a second copy of it.
	sidebarWidth = 22

	// gutterWidth is the floor: the old line number (4), a space, the new
	// line number (4), the +/- marker (1) and the space that separates it
	// from the text (1). It is a floor, not a fixed size: a file whose line
	// numbers run to five digits or more needs a wider gutter, computed by
	// Model.gutter, which the drawing reads to know how much width the text
	// beside it has left.
	gutterWidth = 11

	// headerHeight is the two header lines plus the rule and "Files" heading
	// under them. It is a constant, not a computed length, so the mouse
	// hit-test can read it without laying the screen out a second time.
	headerHeight = 3

	// keyBarHeight is the single line at the bottom of the screen.
	keyBarHeight = 1

	// composerRows is the line-comment composer's fixed textarea height.
	composerRows = 3

	// minWidthForSidebar is where the file list stops earning its columns.
	// Below it the body would be 46 columns, which is 23 Japanese
	// characters. It matches the width at which the Work board drops its
	// card borders.
	minWidthForSidebar = 100
)

// showSidebar reports whether the file list is drawn at all. The mouse
// hit-test reads this too, so the threshold lives in one place.
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
	if m.declined != "" {
		lines = append(lines, m.declinedLine())
	}
	if m.mode == modeCompose {
		lines = append(lines, m.composerLines()...)
	}
	if m.mode == modeSubmit {
		lines = append(lines, m.submitLines()...)
	}
	if m.mode == modeDiscard {
		lines = append(lines, m.discardLines()...)
	}
	return strings.Join(append(lines, m.keyBar()), "\n")
}

func (m Model) keyBar() string {
	switch m.mode {
	case modeCompose:
		return theme.Dim().Render(clip(i18n.T("footer.diff_comment"), m.width))
	case modeSubmit:
		return theme.Dim().Render(clip(i18n.T("footer.submit"), m.width))
	case modeDiscard:
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
	if m.errText != "" {
		lines = append(lines, clip(theme.Error().Render(i18n.T("common.error_prefix"))+singleLine(m.errText), m.width))
	}
	return lines
}

// submitHeight is what submitLines takes, out of the pane's height budget the
// same way composerHeight is, so the popup never pushes the key bar off the
// bottom.
func (m Model) submitHeight() int {
	if m.mode != modeSubmit {
		return 0
	}
	h := 1 + len(strings.Split(m.submit.View(), "\n"))
	if m.errText != "" {
		h++
	}
	return h
}

// discardLines draws the discard confirmation: the question, and either the
// spinner while DiscardReview is running or a failure underneath it.
func (m Model) discardLines() []string {
	lines := []string{"", clip(theme.Error().Render(i18n.T("submit.discard_confirm")), m.width)}
	switch {
	case m.errText != "":
		lines = append(lines, clip(theme.Error().Render(i18n.T("common.error_prefix"))+singleLine(m.errText), m.width))
	case m.phase == phaseWorking:
		lines = append(lines, clip(m.spin.View()+" "+i18n.T("confirm.working"), m.width))
	}
	return lines
}

// discardHeight is what discardLines takes, out of the pane's height budget
// the same way submitHeight is.
func (m Model) discardHeight() int {
	if m.mode != modeDiscard {
		return 0
	}
	h := 2
	if m.errText != "" || m.phase == phaseWorking {
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
	case m.errText != "":
		lines = append(lines, clip(theme.Error().Render(i18n.T("common.error_prefix"))+singleLine(m.errText), m.width))
	case m.phase == phaseWorking:
		lines = append(lines, clip(m.spin.View()+" "+i18n.T("diff.posting"), m.width))
	}
	return lines
}

// composerHeight is what composerLines takes, out of the pane's height
// budget the same way reviewErrHeight is, so the composer never pushes the
// key bar off the bottom.
func (m Model) composerHeight() int {
	if m.mode != modeCompose {
		return 0
	}
	h := 1 + composerRows
	if m.errText != "" || m.phase == phaseWorking {
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

// declinedLine reports why c, v or X just did nothing: the row has no line,
// the review context has not arrived yet, or (X) there is no pending review.
// It is drawn the same way reviewErrLine is -- one line above the key bar,
// dim rather than red, since it is not a failure -- rather than leaving the
// screen unchanged the way the silent guards used to (.claude/rules/errors.md
// applies the same "say why" rule to a declined key as to a failed one).
func (m Model) declinedLine() string {
	return clip(theme.Dim().Render(m.declined), m.width)
}

// declinedHeight is the extra line declinedLine takes, out of the pane's
// height budget the same way reviewErrHeight is.
func (m Model) declinedHeight() int {
	if m.declined != "" {
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
	left := layout.Fill(theme.Heading().Render(i18n.T("diff.files")), sidebarWidth)
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
	return max(m.height-headerHeight-keyBarHeight-m.reviewErrHeight()-m.declinedHeight()-m.composerHeight()-m.submitHeight()-m.discardHeight(), 1)
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
		lines[i] = layout.Fill(left, sidebarWidth) + div + right
	}
	return lines
}

// sidebarLines draws the file list: a path per file, truncated from the
// right when it does not fit, and the size of that file's change under it,
// followed by the count of review threads on that file. It starts at
// m.fileTop, which followSidebar keeps in step with the selected file, the
// same way m.top keeps the diff pane's cursor on screen.
func (m Model) sidebarLines() []string {
	if len(m.files) == 0 {
		return nil
	}
	// body only draws paneHeight lines, and a file takes two of them. Going
	// past that built rows nobody sees -- three lipgloss renders and a walk
	// of every review thread, per file, on every frame.
	h := m.paneHeight()
	lines := make([]string, 0, h)
	for i := m.fileTop; i < len(m.files) && len(lines) < h; i++ {
		f := m.files[i]
		path := clip(f.Path, sidebarWidth)
		plainSize := fmt.Sprintf("+%d −%d", f.Additions, f.Deletions)
		count, pending := m.threadCount(f.Path)
		badge := ""
		if count > 0 {
			badge = fmt.Sprintf("%s%d", icon.ThreadBadge(), count)
		}
		size := theme.Added().Render("+"+strconv.Itoa(f.Additions)) +
			" " + theme.Removed().Render("−"+strconv.Itoa(f.Deletions))
		// The fit is decided on plainSize and badge, which have no colour in
		// them, while what it lets through is the coloured size.
		// ansi.StringWidth ignores colour, so the two agree.
		if badge != "" && ansi.StringWidth(plainSize)+1+ansi.StringWidth(badge) <= sidebarWidth {
			size += " " + theme.Count(pending).Render(badge)
		}
		if i == m.file && m.sidebar {
			lines = append(lines,
				theme.SelectedLine(layout.Fill(path, sidebarWidth)),
				theme.SelectedLine(layout.Fill(size, sidebarWidth)))
			continue
		}
		lines = append(lines, path, size)
	}
	return lines
}

// threadCount reports how many review threads sit on a file, and whether any
// of them is pending: a count the reviewer has not submitted yet draws in
// the colour that says so (theme.Count).
func (m Model) threadCount(path string) (count int, pending bool) {
	for _, t := range m.review.Threads {
		if t.Path != path {
			continue
		}
		count++
		if t.Pending() {
			pending = true
		}
	}
	return count, pending
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

// diffLine draws one row of the diff pane. The cursor row is drawn exactly
// as an unselected one and then filled by theme.SelectedLine, which carries
// the fill past the resets that lipgloss and chroma leave behind; an added
// or removed row elsewhere is filled the same way in its own colour. The
// two never stack: a row carrying the added fill and then wrapped in the
// selection's would keep the added background, since a background is not a
// reset for the selection's fill to carry past, so the cursor wins outright.
func (m Model) diffLine(r row, selected bool, width int) string {
	line := m.styledLine(r, width)
	switch {
	case selected:
		return theme.SelectedLine(layout.Fill(line, width))
	case r.kind == rowLine && r.line.Kind != domain.LineContext:
		// layout.Fill is needed here for the same reason it is above: a
		// background lands only on columns that have a character in them.
		return theme.DiffLine(r.line.Kind, layout.Fill(line, width))
	default:
		return line
	}
}

// styledLine is one row in its own colours, whether or not the cursor is on
// it.
func (m Model) styledLine(r row, width int) string {
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
func (m Model) diffTextLine(l domain.DiffLine, width int) string {
	old, num := lineNumbers(l)
	fw := m.lineNumberWidth()
	body := theme.Highlight(m.currentPath(), clip(l.Text, max(width-m.gutter(), 0)))
	gutter := theme.LineNumber().Render(fmt.Sprintf("%*s %*s", fw, old, fw, num)) +
		markerStyle(l.Kind) + " "
	return gutter + body
}

// gutter is the columns the line numbers and marker occupy: two
// lineNumberWidth fields, the space between them, the marker and the space
// that separates it from the text.
func (m Model) gutter() int { return 2*m.lineNumberWidth() + 3 }

// lineNumberWidth is the gutter's number field, counted when the rows were
// built (countLineNumberWidth). The floor is applied here too, so a Model
// whose rows have not been built yet still measures the same as an empty
// file rather than zero.
func (m Model) lineNumberWidth() int { return max(m.numWidth, (gutterWidth-3)/2) }

// currentPath is the file the cursor is in, used to pick the syntax
// highlighter's lexer.
func (m Model) currentPath() string {
	if len(m.files) == 0 {
		return ""
	}
	return m.files[m.file].Path
}

// lineNumbers is a line's old and new numbers, blank on the side the line
// does not exist on.
func lineNumbers(l domain.DiffLine) (old, num string) {
	if l.OldLine > 0 {
		old = strconv.Itoa(l.OldLine)
	}
	if l.NewLine > 0 {
		num = strconv.Itoa(l.NewLine)
	}
	return old, num
}

// markerStyle colours the +/- marker.
func markerStyle(k domain.DiffLineKind) string {
	switch k {
	case domain.LineAdded:
		return theme.DiffAdded().Render("+")
	case domain.LineRemoved:
		return theme.DiffRemoved().Render("-")
	default:
		return " "
	}
}

// tabWidth is how many columns expandTabs turns a tab into. It matches
// lipgloss's own default, so a row that goes through Style.Render and one
// that does not (chroma's formatter, which leaves a tab as a literal byte)
// measure the same either way.
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
