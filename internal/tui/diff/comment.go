package diff

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

type (
	// commentPostedMsg carries the review id the post went through, so
	// Update can set m.review.PendingID synchronously, before the refetch
	// it also triggers comes back. The refetch is asynchronous; without this
	// field, a second c sent before it lands would still see PendingID
	// empty and call StartReview again, leaving two pending reviews open on
	// the pull request.
	commentPostedMsg struct {
		ref      gh.ItemRef
		reviewID string
	}
	commentErrorMsg struct {
		ref gh.ItemRef
		err error
	}
)

// startComposing opens the composer on the line under the cursor.
//
// A hunk header, a thread and a note have no line to comment on, so c does
// nothing there rather than posting somewhere arbitrary. Neither does it open
// before the review context has arrived: the diff and the context are fetched
// in parallel, and starting a review needs the pull request's node id, which
// only the context carries. Both declines say why at footer level
// (m.declined) rather than leaving the screen unchanged -- that silence is
// what made c look broken in the first place. A context that failed outright
// is the one exception: reviewErr already says so on its own line, so this
// adds nothing on top of it.
//
// The target line and side are captured here, not read again at send time.
// A refetch that lands mid-composition rebuilds m.rows and can insert thread
// rows under the very line being commented on, shifting m.row to a different
// row -- one whose zero gh.DiffLine would otherwise read as line 0. The
// cursor is guaranteed to be on a real line only right now, when the guard
// above has just confirmed it.
func (m Model) startComposing() Model {
	r := m.currentRow()
	switch {
	case r.kind != rowLine:
		m.declined = i18n.T("diff.decline_no_line")
		return m
	case m.reviewErr != nil:
		return m
	case m.review.PullRequestID == "":
		m.declined = i18n.T("diff.decline_loading")
		return m
	}
	m.declined = ""
	line, side := r.line.Line()
	m.target = gh.PendingComment{Path: m.files[m.file].Path, Line: line, Side: side}
	m.composing = true
	m.postErr = ""
	m.textarea.Reset()
	m.textarea.Focus()
	return m
}

// post sends the composed comment, against the target startComposing
// captured -- not whatever row the cursor now sits on. The review is started
// here rather than when the view opens: a diff the user only reads must not
// leave an empty pending review behind on the pull request.
func (m Model) post() (Model, tea.Cmd) {
	body := m.textarea.Value()
	if body == "" {
		return m, nil
	}
	comment := m.target
	comment.Body = body

	src, ref, pullRequestID, reviewID := m.src, m.ref, m.review.PullRequestID, m.review.PendingID
	m.composing = false
	m.posting = true
	m.postErr = ""
	return m, func() tea.Msg {
		if reviewID == "" {
			id, err := src.StartReview(pullRequestID)
			if err != nil {
				return commentErrorMsg{ref: ref, err: err}
			}
			reviewID = id
		}
		if err := src.AddReviewThread(reviewID, comment); err != nil {
			return commentErrorMsg{ref: ref, err: err}
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
		m.target = gh.PendingComment{}
		m.textarea.Reset()
		return m, nil
	case "ctrl+s":
		return m.post()
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}
