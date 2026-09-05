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
	if m.picking {
		return m.pickerView()
	}
	if m.loading || m.pickerLoading {
		return layout.ClipLines(m.spin.View()+" "+i18n.T("common.loading")+"\n", m.width)
	}
	header := theme.Title().Render(m.title)
	footer := theme.Dim().Render(i18n.T("footer.detail_prefix") + m.stateFooterKey() + i18n.T("footer.detail_suffix"))
	body := layout.ClipLines(header, m.width) + "\n" + m.body.View() + "\n"
	if m.actionErr != "" {
		body += wrapErr(m.actionErr, m.width) + "\n"
	}
	return body + layout.ClipLines(footer, m.width)
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
	state := pr.State
	if pr.IsDraft {
		state += i18n.T("md.draft_suffix")
	}
	fmt.Fprintf(&b, "- **%s**: %s\n", i18n.T("md.state"), state)
	if pr.ReviewDecision != "" {
		fmt.Fprintf(&b, "- **%s**: %s\n", i18n.T("md.review"), pr.ReviewDecision)
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
	fmt.Fprintf(&b, "- **%s**: %s\n", i18n.T("md.state"), issue.State)
	writeCommonMeta(&b, issue.Labels, issue.UpdatedAt)
	writeBody(&b, issue.Body)
	writeComments(&b, issue.Comments)
	return b.String()
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
