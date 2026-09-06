package diff

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/review"
)

// discardedMsg carries DiscardReview's answer for the pull request it was
// sent for, dropped if it lands after the user has left this pull request --
// the same guard every other async answer in this package uses.
type discardedMsg struct {
	ref gh.ItemRef
	err error
}

// openSubmit opens the review popup, whether or not anything is waiting:
// approving a diff the viewer had nothing to say about is the commonest
// review there is. It does nothing before the review context has arrived --
// the diff and the context are fetched in parallel, and submitting needs the
// pull request's node id, which only the context carries. Same decline
// treatment as c (comment.go): say why at footer level, except when reviewErr
// already does.
func (m Model) openSubmit() Model {
	if m.review.PullRequestID == "" {
		if m.reviewErr == nil {
			m.declined = i18n.T("diff.decline_loading")
		}
		return m
	}
	m.declined = ""
	target := review.Target{
		PullRequestID:   m.review.PullRequestID,
		PendingID:       m.review.PendingID,
		PendingComments: m.pendingCount(),
	}
	m.submit = review.New(m.src, target)
	m.submit, _ = m.submit.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.submitting = true
	m.submitErr = ""
	return m
}

func (m Model) handleSubmitKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.submit, cmd = m.submit.Update(msg)
	return m, cmd
}

// startDiscard asks before X throws a pending review away: one keystroke is
// too little between a written review and no review. It does nothing when
// there is no pending review to discard, and says so at footer level rather
// than leaving the screen unchanged.
//
// PendingID == "" is ambiguous on its own: it also means "not known yet",
// during the ~6.5s the review context takes to load, or after a refetch has
// failed and left the last confirmed state in place. Only once PullRequestID
// is known does an empty PendingID mean what X should call "no pending
// review" -- so that check comes first, mirroring openSubmit's shape.
func (m Model) startDiscard() Model {
	switch {
	case m.review.PullRequestID == "":
		if m.reviewErr == nil {
			m.declined = i18n.T("diff.decline_loading")
		}
		return m
	case m.review.PendingID == "":
		m.declined = i18n.T("diff.decline_no_pending_review")
		return m
	}
	m.declined = ""
	m.discarding = true
	m.discardErr = ""
	return m
}

func (m Model) handleDiscardKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.discardWorking {
		return m, nil // ignore every other key while the discard is in flight
	}
	switch msg.String() {
	case "y":
		return m.discard()
	case "n", "esc":
		m.discarding = false
		m.discardErr = ""
		return m, nil
	}
	return m, nil
}

func (m Model) discard() (Model, tea.Cmd) {
	src, ref, reviewID := m.src, m.ref, m.review.PendingID
	m.discardWorking = true
	m.discardErr = ""
	return m, func() tea.Msg {
		return discardedMsg{ref: ref, err: src.DiscardReview(reviewID)}
	}
}
