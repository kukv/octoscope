// Package review is the popup that submits a pull request review: which of
// the three things it says, and the note that goes with it.
//
// It is a popup rather than a view of its own, so it has no place in the root
// model's stack. The diff view and the detail view each hold one and draw it
// over themselves.
package review

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/usecase"
)

// Source is what submitting needs. Which GitHub call that takes -- the
// pending review's submit or a create-and-submit in one -- is GitHub's rule
// about unsubmitted reviews, not a question about this popup.
type Source interface {
	SubmitReview(t usecase.ReviewTarget, event gh.ReviewEvent, body string) error
}

// Target names what the popup submits against: the pull request it belongs
// to, the unsubmitted review if there is one, and how many line comments are
// waiting to go out with it.
type Target struct {
	PullRequestID   string
	PendingID       string
	PendingComments int
}

// SubmittedMsg tells the holder the review went out; it should refetch.
type SubmittedMsg struct{}

// CancelledMsg tells the holder to take the popup away.
type CancelledMsg struct{}

// ErrorMsg carries a failure the holder shows at footer level. The popup
// keeps no error text of its own -- the event chosen and the note typed so
// far live only in this Model, and the holder is what decides where a
// failure is drawn (.claude/rules/errors.md).
type ErrorMsg struct{ Err error }

// composerRows is the note textarea's fixed height.
const composerRows = 3

// boxWidth is the popup's own cap, chosen to read comfortably without
// spanning a wide terminal; boxWidth never exceeds what width actually
// leaves room for, so the box fits even at 80 columns.
const boxWidth = 50

type Model struct {
	src    Source
	target Target
	event  gh.ReviewEvent

	textarea textarea.Model
	sending  bool

	width, height int
}

// New builds the popup, opened on EventComment: the mildest of the three, so
// tab has somewhere to start from that sends nothing more than a note.
func New(src Source, target Target) Model {
	ta := textarea.New()
	ta.Placeholder = i18n.T("submit.placeholder")
	ta.ShowLineNumbers = false
	ta.SetHeight(composerRows)
	ta.Focus()
	return Model{src: src, target: target, textarea: ta}
}

// Active reports whether the popup has anything to show. A zero Model,
// before New has built it, reports false.
func (m Model) Active() bool { return m.target.PullRequestID != "" }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.textarea.SetWidth(max(m.boxWidth()-2, 0))
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case ErrorMsg:
		// The submission failed; the holder records the text, this only
		// clears the in-flight flag so the popup accepts keys again.
		m.sending = false
		return m, nil
	case SubmittedMsg:
		m.sending = false
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.sending {
		return m, nil // ignore every other key while the review is in flight
	}
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return CancelledMsg{} }
	case "tab":
		m.event = nextEvent(m.event)
		return m, nil
	case "ctrl+s":
		return m.submit()
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// nextEvent walks tab through comment, approve and request changes, in that
// order, back to comment.
func nextEvent(e gh.ReviewEvent) gh.ReviewEvent {
	switch e {
	case gh.EventComment:
		return gh.EventApprove
	case gh.EventApprove:
		return gh.EventRequestChanges
	default:
		return gh.EventComment
	}
}

func (m Model) submit() (Model, tea.Cmd) {
	src := m.src
	target := usecase.ReviewTarget{
		PullRequestID: m.target.PullRequestID,
		PendingID:     m.target.PendingID,
	}
	event, body := m.event, m.textarea.Value()
	m.sending = true
	return m, func() tea.Msg {
		if err := src.SubmitReview(target, event, body); err != nil {
			return ErrorMsg{Err: err}
		}
		return SubmittedMsg{}
	}
}

// boxWidth is the popup's outer content width: never wider than boxWidth
// columns, and never wider than the terminal actually leaves room for.
func (m Model) boxWidth() int { return min(boxWidth, max(m.width-4, 0)) }
