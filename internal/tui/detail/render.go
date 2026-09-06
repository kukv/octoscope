package detail

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

func (m Model) View() string {
	if m.composing {
		return m.composeView()
	}
	if m.confirming {
		return m.confirmView()
	}
	if m.submitting {
		return m.submitView()
	}
	if m.picking {
		return m.pickerView()
	}
	if m.loading || m.pickerLoading || m.openingReview {
		return layout.ClipLines(m.spin.View()+" "+i18n.T("common.loading")+"\n", m.width)
	}
	header := theme.Title().Render(m.title)
	footer := theme.Dim().Render(m.footer())
	body := layout.ClipLines(header, m.width) + "\n" + m.body.View() + "\n"
	if m.actionErr != "" {
		body += wrapErr(m.actionErr, m.width) + "\n"
	}
	return body + layout.ClipLines(footer, m.width)
}

// footer builds the detail view's key bar from hints, most important first,
// dropping from the low-priority end when the terminal is too narrow for all
// of them -- the same mechanism diff's render.go uses for its own key bar.
func (m Model) footer() string {
	return fitKeyBar(m.footerHints(), m.width)
}

// footerHints lists the detail view's hints, most important first. esc is
// first because fitKeyBar never drops it: it is the only way out of the
// view. review and diff only apply to a pull request, and state (close or
// reopen) only when the item can do one of them (not merged).
func (m Model) footerHints() []string {
	hints := []string{
		i18n.T("footer.detail.esc"),
		i18n.T("footer.detail.move"),
		i18n.T("footer.detail.comment"),
	}
	if m.ref.Kind == gh.ItemPR {
		hints = append(hints, i18n.T("footer.detail.review"), i18n.T("footer.detail.diff"))
	}
	if s := m.stateFooterKey(); s != "" {
		hints = append(hints, s)
	}
	return append(hints,
		i18n.T("footer.detail.refresh"),
		i18n.T("footer.detail.web"),
		i18n.T("footer.detail.labels"),
		i18n.T("footer.detail.assign"),
	)
}

// fitKeyBar joins hints in order and drops from the low-priority end (the
// tail of the slice) until the joined line fits width. No ellipsis: a bar
// that shows fewer hints cleanly beats one that shows more but cuts one off
// mid-word. hints[0] is always kept, so as long as the caller orders esc
// first, esc is what survives when only one hint fits -- clipped, if even it
// does not fit, rather than left to overrun the width budget.
//
// This is the same mechanism as internal/tui/diff/render.go's fitKeyBar;
// sharing it (in internal/tui/layout, say) would be a reasonable follow-up,
// but this task does not restructure that package without asking first.
func fitKeyBar(hints []string, width int) string {
	if width <= 0 {
		return strings.Join(hints, "  ")
	}
	for n := len(hints); n > 0; n-- {
		joined := strings.Join(hints[:n], "  ")
		if ansi.StringWidth(joined) <= width {
			return joined
		}
	}
	return layout.ClipLines(hints[0], width)
}

// submitView draws the review popup: the title, the popup's own box, and a
// failed submission's error underneath it. The popup keeps no error text of
// its own, so what the user typed and chose is still there for a retry
// (.claude/rules/errors.md).
func (m Model) submitView() string {
	body := layout.ClipLines(theme.Title().Render(m.title), m.width) + "\n\n"
	body += m.submit.View() + "\n"
	if m.submitErr != "" {
		body += wrapErr(m.submitErr, m.width) + "\n"
	}
	return body + layout.ClipLines(theme.Dim().Render(i18n.T("footer.submit")), m.width)
}

// wrapErr lays out a failure that came from gh or GitHub. Unlike the hints and
// titles around it, this text is the whole of what the user has to go on, so
// it is wrapped rather than cut short (.claude/rules/errors.md).
func wrapErr(text string, w int) string {
	s := theme.Error().Render(i18n.T("common.error_prefix")) + text
	if w <= 0 {
		return s
	}
	return ansi.Wrap(s, w, "")
}

func (m Model) pickerView() string {
	body := m.picker.listView(m.height, m.width)
	if m.applying {
		return body + "\n" + layout.ClipLines(m.spin.View()+" "+i18n.T("picker.applying"), m.width) + "\n"
	}
	return body + "\n" + layout.ClipLines(theme.Dim().Render(i18n.T("footer.picker")), m.width)
}

// stateFooterKey returns the state-aware footer hint (with trailing spaces),
// or "" when the item cannot change state (merged / not yet loaded).
func (m Model) stateFooterKey() string {
	closing, ok := m.stateAction()
	if !ok {
		return ""
	}
	if closing {
		return i18n.T("footer.close")
	}
	return i18n.T("footer.reopen")
}

func (m Model) confirmView() string {
	header := theme.Title().Render(m.title)
	closing, _ := m.stateAction()
	var id string
	switch {
	case m.ref.Kind == gh.ItemPR && closing:
		id = "confirm.close_pr"
	case m.ref.Kind == gh.ItemPR:
		id = "confirm.reopen_pr"
	case closing:
		id = "confirm.close_issue"
	default:
		id = "confirm.reopen_issue"
	}
	var b strings.Builder
	b.WriteString(layout.ClipLines(header, m.width) + "\n\n")
	b.WriteString(i18n.T(id))
	if m.working {
		b.WriteString(m.spin.View() + " " + i18n.T("confirm.working") + "\n")
	} else {
		b.WriteString(theme.Dim().Render(i18n.T("confirm.yes_no")))
	}
	return layout.ClipLines(b.String(), m.width)
}

func (m Model) composeView() string {
	var b strings.Builder
	title := theme.Title().Render(i18n.Tf("compose.title", map[string]any{"Title": m.title}))
	b.WriteString(layout.ClipLines(title, m.width) + "\n\n")
	b.WriteString(m.textarea.View() + "\n\n")
	if m.postErr != "" {
		b.WriteString(wrapErr(m.postErr, m.width) + "\n\n")
	}
	if m.posting {
		b.WriteString(layout.ClipLines(m.spin.View()+" "+i18n.T("compose.posting"), m.width) + "\n")
	} else {
		b.WriteString(layout.ClipLines(theme.Dim().Render(i18n.T("footer.compose")), m.width))
	}
	return b.String()
}

func cursorPrefix(selected bool) string {
	if selected {
		return "▸ "
	}
	return "  "
}

func prMarkdown(pr gh.PR) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# #%d %s\n\n", pr.Number, pr.Title)
	fmt.Fprintf(&b, "- **%s**: @%s\n", i18n.T("md.author"), pr.Author.Login)
	state := stateText(pr.State)
	if pr.IsDraft {
		state += i18n.T("md.draft_suffix")
	}
	fmt.Fprintf(&b, "- **%s**: %s\n", i18n.T("md.state"), state)
	if pr.Review != gh.ReviewNone {
		fmt.Fprintf(&b, "- **%s**: %s\n", i18n.T("md.review"), reviewText(pr.Review))
	}
	writeCommonMeta(&b, pr.Labels, pr.UpdatedAt)
	writeBody(&b, pr.Body)
	writeComments(&b, pr.Comments)
	return b.String()
}

func issueMarkdown(issue gh.Issue) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# #%d %s\n\n", issue.Number, issue.Title)
	fmt.Fprintf(&b, "- **%s**: @%s\n", i18n.T("md.author"), issue.Author.Login)
	fmt.Fprintf(&b, "- **%s**: %s\n", i18n.T("md.state"), stateText(issue.State))
	writeCommonMeta(&b, issue.Labels, issue.UpdatedAt)
	writeBody(&b, issue.Body)
	writeComments(&b, issue.Comments)
	return b.String()
}

// stateText and reviewText name a state in the reader's language. GitHub's
// own spelling stopped at the access layer (.claude/rules/architecture.md),
// and a state word is ours to translate (spec 6.1).
func stateText(s gh.ItemState) string {
	switch s {
	case gh.StateOpen:
		return i18n.T("state.open")
	case gh.StateMerged:
		return i18n.T("state.merged")
	default:
		return i18n.T("state.closed")
	}
}

func reviewText(r gh.ReviewState) string {
	switch r {
	case gh.ReviewApproved:
		return i18n.T("review.approved")
	case gh.ReviewChangesRequested:
		return i18n.T("review.changes_requested")
	case gh.ReviewRequired:
		return i18n.T("review.required")
	default:
		return i18n.T("review.none")
	}
}

func writeCommonMeta(b *strings.Builder, labels []gh.Label, updatedAt time.Time) {
	if len(labels) > 0 {
		names := make([]string, len(labels))
		for i, l := range labels {
			names[i] = l.Name
		}
		fmt.Fprintf(b, "- **%s**: %s\n", i18n.T("md.labels"), strings.Join(names, ", "))
	}
	fmt.Fprintf(b, "- **%s**: %s\n", i18n.T("md.updated"), i18n.DateTime(updatedAt))
}

func writeBody(b *strings.Builder, body string) {
	b.WriteString("\n---\n\n")
	if body != "" {
		b.WriteString(body)
	} else {
		b.WriteString(i18n.T("md.no_description"))
	}
}

func writeComments(b *strings.Builder, comments []gh.Comment) {
	for _, c := range comments {
		fmt.Fprintf(b, "\n\n---\n\n**@%s** — %s\n\n%s",
			c.Author.Login, i18n.DateTime(c.CreatedAt), c.Body)
	}
}
