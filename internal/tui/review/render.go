package review

import (
	"strings"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// View draws the popup's own box: the title, which of the three things it
// says, the note, and how many line comments go with it. The holder places
// this over what it already draws; View knows nothing about that.
func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(theme.Title().Render(i18n.T("submit.title")) + "\n\n")
	b.WriteString(m.eventLine() + "\n\n")
	b.WriteString(m.textarea.View() + "\n\n")
	b.WriteString(m.pendingLine())
	if m.sending {
		b.WriteString("\n" + i18n.T("submit.sending"))
	}
	return theme.Popup().Width(m.boxWidth()).Render(b.String())
}

// eventLine shows the three things a review can say, with the one tab would
// leave it on picked out.
func (m Model) eventLine() string {
	options := []struct {
		event gh.ReviewEvent
		text  string
	}{
		{gh.EventComment, i18n.T("submit.comment")},
		{gh.EventApprove, i18n.T("submit.approve")},
		{gh.EventRequestChanges, i18n.T("submit.request_changes")},
	}
	parts := make([]string, len(options))
	for i, o := range options {
		if o.event == m.event {
			parts[i] = theme.Selected().Render(o.text)
			continue
		}
		parts[i] = o.text
	}
	return strings.Join(parts, "  ")
}

// pendingLine says how many line comments are waiting to go out with the
// review, or that none are.
func (m Model) pendingLine() string {
	if m.target.PendingComments == 0 {
		return theme.Dim().Render(i18n.T("submit.no_pending"))
	}
	return theme.Dim().Render(i18n.Tn("submit.pending_count", m.target.PendingComments))
}
