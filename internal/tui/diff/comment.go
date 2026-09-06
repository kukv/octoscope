package diff

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type (
	// commentPostedMsg carries the review id the post went through, so
	// Update can let the next comment reuse it without waiting on a
	// refetch of the review context.
	commentPostedMsg struct {
		ref      gh.ItemRef
		reviewID string
	}
	commentErrorMsg struct{ err error }
)

// startComposing opens the composer on the line under the cursor.
//
// A hunk header, a thread and a note have no line to comment on, so c does
// nothing there rather than posting somewhere arbitrary. Neither does it open
// before the review context has arrived: the diff and the context are fetched
// in parallel, and starting a review needs the pull request's node id, which
// only the context carries.
func (m Model) startComposing() Model {
	r := m.currentRow()
	if r.kind != rowLine || m.review.PullRequestID == "" {
		return m
	}
	m.composing = true
	m.postErr = ""
	m.textarea.Reset()
	m.textarea.Focus()
	return m
}

// post sends the composed comment. The review is started here rather than
// when the view opens: a diff the user only reads must not leave an empty
// pending review behind on the pull request.
func (m Model) post() (Model, tea.Cmd) {
	body := m.textarea.Value()
	if body == "" {
		return m, nil
	}
	line, side := m.currentRow().line.Line()
	comment := gh.PendingComment{
		Path: m.files[m.file].Path,
		Line: line,
		Side: side,
		Body: body,
	}

	src, ref, pullRequestID, reviewID := m.src, m.ref, m.review.PullRequestID, m.review.PendingID
	m.composing = false
	m.posting = true
	m.postErr = ""
	return m, func() tea.Msg {
		if reviewID == "" {
			id, err := src.StartReview(pullRequestID)
			if err != nil {
				return commentErrorMsg{err}
			}
			reviewID = id
		}
		if err := src.AddReviewThread(reviewID, comment); err != nil {
			return commentErrorMsg{err}
		}
		return commentPostedMsg{ref: ref, reviewID: reviewID}
	}
}

// handleComposeKey is the composer's own key handling, the same shape as
// detail's: ctrl+s sends, esc discards the draft, everything else goes to
// the textarea.
func (m Model) handleComposeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.posting {
		return m, nil // ignore every other key while the comment is in flight
	}
	switch msg.String() {
	case "esc":
		m.composing = false
		m.postErr = ""
		m.textarea.Reset()
		return m, nil
	case "ctrl+s":
		return m.post()
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}
