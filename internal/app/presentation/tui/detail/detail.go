// Package detail shows one pull request or issue: its body and comments, the
// comment composer, the close/reopen confirmation and the label/assignee
// picker.
package detail

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/merge"
	"github.com/kukv/octoscope/internal/app/presentation/tui/review"
	"github.com/kukv/octoscope/internal/app/usecase"
	"github.com/kukv/octoscope/internal/browser"
	"github.com/kukv/octoscope/internal/i18n"
)

// chromeLines is what the body never gets. Two of it is exact -- the title
// line above and the key bar below -- and the other two are slack, so that an
// error long enough to wrap onto a second line still does not push the key bar
// off the bottom of the screen.
const chromeLines = 4

type itemSource interface {
	GetItem(ctx context.Context, ref domain.ItemRef) (usecase.Item, error)
	AddComment(ref domain.ItemRef, body string) error
	SetState(ref domain.ItemRef, closing bool) error
	EditLabels(ref domain.ItemRef, add, remove []string) error
	EditAssignees(ref domain.ItemRef, add, remove []string) error
}

// candidateSource lists what a picker offers. Labels and assignees belong to
// the repository, not to a PR or an issue.
type candidateSource interface {
	ListLabels(ctx context.Context, repo string) ([]domain.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

type reviewOpener interface {
	PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error)
}

// Source is what the detail view needs. repo is "owner/repo"; the empty
// string targets the workspace repository.
type Source interface {
	itemSource
	candidateSource
	reviewOpener
	review.Source
	merge.Source
}

// ClosedMsg tells the parent the user left the detail view.
type ClosedMsg struct{}

// OpenDiffMsg asks the parent to show the diff of the shown pull request.
type OpenDiffMsg struct{ Ref domain.ItemRef }

// OpenChecksMsg asks the parent to show the checks of the shown pull request.
type OpenChecksMsg struct{ Ref domain.ItemRef }

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type (
	// itemMsg carries the ref because the request for the item the user
	// just left is still running, and its answer must not land here.
	itemMsg struct {
		ref  domain.ItemRef
		item usecase.Item
	}
	errMsg struct {
		ref domain.ItemRef
		err error
	}
	// The comment and state answers carry the ref for the same reason
	// itemMsg does. Both end in a refetch of the item they were sent for,
	// which on another item would replace what the user is reading.
	commentPostedMsg struct{ ref domain.ItemRef }
	commentErrorMsg  struct {
		ref domain.ItemRef
		err error
	}
	stateChangedMsg struct{ ref domain.ItemRef }
	stateErrorMsg   struct {
		ref domain.ItemRef
		err error
	}
	// The three picker answers carry the ref for the same reason itemMsg
	// does. Labels and assignees belong to the repository, so an answer
	// started on another item opens a picker nobody asked for, offering
	// that repository's candidates.
	pickerCandidatesMsg struct {
		ref    domain.ItemRef
		kind   pickerKind
		labels []domain.Label
		users  []string
	}
	pickerAppliedMsg struct{ ref domain.ItemRef }
	pickErrorMsg     struct {
		ref domain.ItemRef
		err error
	}

	// reviewContextMsg and reviewContextErrMsg carry the ref for the same
	// reason itemMsg does.
	reviewContextMsg struct {
		ref domain.ItemRef
		ctx domain.ReviewContext
	}
	reviewContextErrMsg struct {
		ref domain.ItemRef
		err error
	}
)

// mode is which overlay is on screen (.claude/rules/tui.md).
type mode uint8

const (
	modeView mode = iota
	modeCompose
	modeConfirm
	modePick
	modeSubmit
	modeMerge
)

// String names the mode wherever it is printed. Every assertion on the state
// of this view reports it with %v, and "mode = 3" leaves the reader counting
// the constants to find out which overlay that was.
func (m mode) String() string {
	switch m {
	case modeView:
		return "view"
	case modeCompose:
		return "compose"
	case modeConfirm:
		return "confirm"
	case modePick:
		return "pick"
	case modeSubmit:
		return "submit"
	case modeMerge:
		return "merge"
	}
	return "mode(?)"
}

// phase is where the current mode is in its round trip. modeSubmit never
// reaches phaseWorking: review.Model owns the send. modeMerge stays at
// phaseIdle, because merge.Model owns its fetch as well as its send.
type phase uint8

const (
	phaseIdle phase = iota
	phaseLoading
	phaseWorking
)

func (p phase) String() string {
	switch p {
	case phaseIdle:
		return "idle"
	case phaseLoading:
		return "loading"
	case phaseWorking:
		return "working"
	}
	return "phase(?)"
}

type Model struct {
	src Source
	ref domain.ItemRef

	// viewer is the login of the signed-in user, or "" when it is not known
	// -- the lookup has not answered yet, or it failed. Empty draws the body
	// exactly as it was drawn before mentions were highlighted at all.
	viewer string

	width, height int

	mode  mode
	phase phase

	// errText is the last failure; the mode decides where it is drawn.
	// Opening an overlay clears it rather than draw the body's failure
	// inside it.
	errText string

	// declined says why the last key did nothing, in the one case where the
	// screen cannot show it: while the item is loading there is a spinner
	// and nothing else, so c, x, v, l and a look broken rather than early.
	declined string

	spin  spinner.Model
	body  viewport.Model
	title string
	state domain.ItemState
	url   string

	// item is the whole of what was fetched. The meta pane reads it at draw
	// time, so unlike title and state it cannot be turned into a string once
	// and thrown away: its layout changes with the terminal's width.
	//
	// loaded is the invariant the drawing relies on: item is the fetched
	// value only once it is true, and every reader of item -- rows,
	// headerLines, setBodyContent -- draws nothing until then, rather than
	// drawing the zero item as an unnamed, unauthored, never-updated one.
	item   usecase.Item
	loaded bool

	textarea textarea.Model

	picker    picker
	labels    []string
	assignees []string

	submit review.Model
	merge  merge.Model

	// open shows a URL. It is browser.Open outside tests: opening a page is
	// not a GitHub call, so it does not go through the backend.
	open func(url string) error
}

func New(src Source, ref domain.ItemRef) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	ta := textarea.New()
	ta.Placeholder = i18n.T("compose.placeholder")
	ta.ShowLineNumbers = false
	return Model{
		src:      src,
		ref:      ref,
		phase:    phaseLoading,
		spin:     s,
		body:     newBody(),
		textarea: ta,
		open:     browser.Open,
	}
}

// SetViewer names the signed-in user, so the body can tell a comment
// addressed at the reader from one that is not. The root calls it as the
// view is built; search.New(src).SetSavedQueries(...) is the same shape.
//
// It is not a parameter of New because the login is the one input that may
// not have arrived yet, and every test that builds a detail view would
// otherwise have to say it does not care.
func (m Model) SetViewer(login string) Model {
	m.viewer = login
	return m
}

// ViewerForTest is what the root told this view. It exists so that the
// root's test can see the login arrive without reaching into another
// package's fields.
func (m Model) ViewerForTest() string { return m.viewer }

// newBody is the scrolling body pane. The wheel has to be turned on for the
// viewport to act on it; the root model is what asks the terminal to report
// mouse events at all.
func newBody() viewport.Model {
	v := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	v.MouseWheelEnabled = true
	// The rule between the panes runs as far as the body does, so the body
	// has to be as tall as the pane even when the item is short.
	v.FillHeight = true
	return v
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetch(m.src, m.ref))
}

// stateAction reports whether the shown item can change state, and if so
// whether the action is a close (true) or a reopen (false).
func (m Model) stateAction() (closing bool, ok bool) {
	if m.phase == phaseLoading {
		return false, false
	}
	switch m.state {
	case domain.StateOpen:
		return true, true
	case domain.StateClosed:
		return false, true
	default:
		return false, false // merged: neither closing nor reopening applies
	}
}

// canMerge reports whether the merge key applies: only an open pull request
// can be merged, and until the item has arrived the state is not known.
func (m Model) canMerge() bool {
	closing, ok := m.stateAction()
	return m.ref.Kind == domain.ItemPR && ok && closing
}

// relayout gives the body its width and height and lays the content out
// again. It runs both on a resize and when the item arrives: what the body
// is worth depends on the width, and — once the meta block sits above it —
// how tall that block turned out.
func (m *Model) relayout() {
	w := m.width
	if w <= 0 {
		w = 80
	}
	h := m.height
	if h <= 0 {
		h = 24
	}
	bodyW, bodyH := w, h-chromeLines
	if twoPane(w) {
		bodyW = w - metaPaneWidth(w) - 1 // the rule JoinPanes draws between them
	} else {
		bodyH -= len(m.headerLines()) // the meta paragraph above the body
	}
	m.body.SetWidth(max(bodyW, 1))
	m.body.SetHeight(max(bodyH, 5))
	m.setBodyContent(max(bodyW, 1))
}

// setBodyContent re-renders the body at w, keeping the reader's place. The
// content is laid out for a width, so every width change rebuilds it.
func (m *Model) setBodyContent(w int) {
	if !m.loaded {
		return
	}
	at := m.body.YOffset()
	m.body.SetContentLines(bodyLines(m.item, w))
	m.body.SetYOffset(at)
}

func labelNames(labels []domain.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}

func authorLogins(authors []domain.Author) []string {
	logins := make([]string, len(authors))
	for i, a := range authors {
		logins[i] = a.Login
	}
	return logins
}
