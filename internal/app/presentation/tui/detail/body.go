package detail

import (
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/app/usecase"
	"github.com/kukv/octoscope/internal/i18n"
)

// bodyIndent is how far the description sits in from the section heading, and
// bodyIndentWidth is what it costs. It replaces the margin glamour would
// otherwise add, which is turned off in markdownLines so that a comment's bar
// has nothing between it and the text.
const (
	bodyIndent      = "  "
	bodyIndentWidth = 2
)

// commentBarWidth is what the bar down the left of a comment takes from the
// text beside it: the glyph and the space after it. The glyph is one column in
// every set (icon's TestEveryGlyphIsOneColumn), whatever it measures in bytes.
const commentBarWidth = 2

// sectionHeading names a section and runs a rule from the end of the name to
// the edge of the pane. The name on its own was quieter than the text beneath
// it -- theme.Heading is muted, and a GitHub body is not -- which left the
// reader looking for the end of the description in a blank line.
func sectionHeading(name string, w int) string {
	head := theme.Heading().Render(name)
	// The space is what keeps the rule from running into the last character.
	rest := w - ansi.StringWidth(name) - 1
	if rest <= 0 {
		return layout.Clip(head, w)
	}
	return head + " " + theme.Rule().Render(strings.Repeat("─", rest))
}

// bodyLines is what scrolls: the description under its heading, then every
// comment behind its own bar.
func bodyLines(it usecase.Item, w int) []string {
	lines := []string{sectionHeading(i18n.T("detail.section.description"), w)}

	body := it.Body
	if strings.TrimSpace(body) == "" {
		body = i18n.T("detail.no_description")
	}
	for _, l := range markdownLines(body, max(w-bodyIndentWidth, 1)) {
		lines = append(lines, bodyIndent+l)
	}

	if len(it.Comments) == 0 {
		return fit(lines, w)
	}
	lines = append(lines, "",
		sectionHeading(i18n.Tn("detail.section.comments", len(it.Comments)), w))
	for _, c := range it.Comments {
		lines = append(lines, "")
		lines = append(lines, commentLines(c, w)...)
	}
	return fit(lines, w)
}

// fit cuts every line to the pane's width. glamour wraps prose, but a long
// URL or a line of code has nowhere to wrap, and one line past the edge
// pushes the rule between the panes sideways for its whole height.
func fit(lines []string, w int) []string {
	for i, l := range lines {
		lines[i] = layout.Clip(l, w)
	}
	return lines
}

// commentLines draws one comment: who wrote it and when, then the body, with
// a bar down the left of every line. The bar is what says where one comment
// ends and the next begins, so it cannot be left off a wrapped line.
func commentLines(c domain.Comment, w int) []string {
	bar := theme.Dim().Render(icon.CommentBar() + " ")
	lines := []string{bar + theme.Dim().Render("@"+c.Author.Login+" · "+i18n.DateTime(c.CreatedAt))}
	for _, l := range markdownLines(c.Body, max(w-commentBarWidth, 1)) {
		lines = append(lines, bar+l)
	}
	return lines
}

// markdownLines renders one block of GitHub markdown to w columns.
//
// The document margin is turned off: glamour would indent every line by two
// columns, which inside a comment would open a gap between the bar and the
// text. The blank lines glamour puts around a document are dropped for the
// same reason — a bar with nothing beside it reads as a break in the comment.
func markdownLines(src string, w int) []string {
	// A GitHub body carries whatever line endings its author used. A stray
	// carriage return inside a drawn line sends the cursor back to the start
	// of it, which shifts everything after it sideways.
	src = strings.ReplaceAll(src, "\r", "")

	// out starts as the source so that a renderer glamour will not build, or a
	// document it will not render, still puts the author's words on the
	// screen. Markdown is readable unrendered, and there is nowhere in a
	// scrolling body to report the failure that would not cost more than it
	// tells the reader.
	out := src
	cfg := styles.DarkStyleConfig
	var noMargin uint
	cfg.Document.Margin = &noMargin
	if r, err := glamour.NewTermRenderer(glamour.WithStyles(cfg),
		glamour.WithWordWrap(max(w, 1))); err == nil {
		if rendered, err := r.Render(src); err == nil {
			out = rendered
		}
	}
	return trimBlankEdges(strings.Split(out, "\n"))
}

// trimBlankEdges drops the blank lines at either end of a rendered block.
func trimBlankEdges(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[len(lines)-1])) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
