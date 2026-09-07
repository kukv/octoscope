// Package detail shows one pull request or issue: its body and comments, the
// comment composer, the close/reopen confirmation and the label/assignee
// picker.
package detail

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/merge"
	"github.com/kukv/octoscope/internal/tui/review"
	"github.com/kukv/octoscope/internal/usecase"
)

type itemSource interface {
	GetItem(ctx context.Context, ref gh.ItemRef) (usecase.Item, error)
	AddComment(ref gh.ItemRef, body string) error
	SetState(ref gh.ItemRef, closing bool) error
	EditLabels(ref gh.ItemRef, add, remove []string) error
	EditAssignees(ref gh.ItemRef, add, remove []string) error
	OpenWeb(url string) error
}

// candidateSource lists what a picker offers. Labels and assignees belong to
// the repository, not to a PR or an issue.
type candidateSource interface {
	ListLabels(ctx context.Context, repo string) ([]gh.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

type reviewOpener interface {
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
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
type OpenDiffMsg struct{ Ref gh.ItemRef }

// OpenChecksMsg asks the parent to show the checks of the shown pull request.
type OpenChecksMsg struct{ Ref gh.ItemRef }

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type (
	// itemMsg carries the ref because the request for the item the user
	// just left is still running, and its answer must not land here.
	itemMsg struct {
		ref  gh.ItemRef
		item usecase.Item
	}
	errMsg struct {
		ref gh.ItemRef
		err error
	}
	// The comment and state answers carry the ref for the same reason
	// itemMsg does. Both end in a refetch of the item they were sent for,
	// which on another item would replace what the user is reading.
	commentPostedMsg struct{ ref gh.ItemRef }
	commentErrorMsg  struct {
		ref gh.ItemRef
		err error
	}
	stateChangedMsg struct{ ref gh.ItemRef }
	stateErrorMsg   struct {
		ref gh.ItemRef
		err error
	}
	// The three picker answers carry the ref for the same reason itemMsg
	// does. Labels and assignees belong to the repository, so an answer
	// started on another item opens a picker nobody asked for, offering
	// that repository's candidates.
	pickerCandidatesMsg struct {
		ref    gh.ItemRef
		kind   pickerKind
		labels []gh.Label
		users  []string
	}
	pickerAppliedMsg struct{ ref gh.ItemRef }
	pickErrorMsg     struct {
		ref gh.ItemRef
		err error
	}

	// reviewContextMsg and reviewContextErrMsg carry the ref for the same
	// reason itemMsg does.
	reviewContextMsg struct {
		ref gh.ItemRef
		ctx gh.ReviewContext
	}
	reviewContextErrMsg struct {
		ref gh.ItemRef
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
	ref gh.ItemRef

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
	state gh.ItemState
	url   string

	textarea textarea.Model

	picker    picker
	labels    []string
	assignees []string

	submit review.Model
	merge  merge.Model
}

func New(src Source, ref gh.ItemRef) Model {
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
	}
}

// newBody is the scrolling body pane. The wheel has to be turned on for the
// viewport to act on it; the root model is what asks the terminal to report
// mouse events at all.
func newBody() viewport.Model {
	v := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	v.MouseWheelEnabled = true
	return v
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetch(m.src, m.ref))
}

func fetch(src Source, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		item, err := src.GetItem(context.Background(), ref)
		if err != nil {
			return errMsg{ref, err}
		}
		return itemMsg{ref, item}
	}
}

// fetchReviewContext is what v runs before it can open the review popup: an
// issue has no review, so this is only ever called on a pull request.
func fetchReviewContext(src reviewOpener, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		ctx, err := src.PRReviewContext(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return reviewContextErrMsg{ref: ref, err: err}
		}
		return reviewContextMsg{ref: ref, ctx: ctx}
	}
}

func openWeb(src Source, ref gh.ItemRef, url string) tea.Cmd {
	return func() tea.Msg {
		if err := src.OpenWeb(url); err != nil {
			return errMsg{ref, err}
		}
		return nil
	}
}

func postComment(src Source, ref gh.ItemRef, body string) tea.Cmd {
	return func() tea.Msg {
		if err := src.AddComment(ref, body); err != nil {
			return commentErrorMsg{ref: ref, err: err}
		}
		return commentPostedMsg{ref: ref}
	}
}

// stateAction reports whether the shown item can change state, and if so
// whether the action is a close (true) or a reopen (false).
func (m Model) stateAction() (closing bool, ok bool) {
	if m.phase == phaseLoading {
		return false, false
	}
	switch m.state {
	case gh.StateOpen:
		return true, true
	case gh.StateClosed:
		return false, true
	default:
		return false, false // merged: neither closing nor reopening applies
	}
}

// canMerge reports whether the merge key applies: only an open pull request
// can be merged, and until the item has arrived the state is not known.
func (m Model) canMerge() bool {
	closing, ok := m.stateAction()
	return m.ref.Kind == gh.ItemPR && ok && closing
}

func setState(src Source, ref gh.ItemRef, closing bool) tea.Cmd {
	return func() tea.Msg {
		if err := src.SetState(ref, closing); err != nil {
			return stateErrorMsg{ref: ref, err: err}
		}
		return stateChangedMsg{ref: ref}
	}
}

func fetchLabelPicker(src candidateSource, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		labels, err := src.ListLabels(context.Background(), ref.Repo)
		if err != nil {
			return pickErrorMsg{ref: ref, err: err}
		}
		return pickerCandidatesMsg{ref: ref, kind: pickLabels, labels: labels}
	}
}

func fetchAssigneePicker(src candidateSource, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		users, err := src.ListAssignees(context.Background(), ref.Repo)
		if err != nil {
			return pickErrorMsg{ref: ref, err: err}
		}
		return pickerCandidatesMsg{ref: ref, kind: pickAssignees, users: users}
	}
}

func applyPicker(src Source, ref gh.ItemRef, kind pickerKind, add, remove []string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if kind == pickLabels {
			err = src.EditLabels(ref, add, remove)
		} else {
			err = src.EditAssignees(ref, add, remove)
		}
		if err != nil {
			return pickErrorMsg{ref: ref, err: err}
		}
		return pickerAppliedMsg{ref: ref}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg), nil
	case spinner.TickMsg:
		return m.tick(msg)
	case itemMsg:
		return m.itemArrived(msg), nil
	case commentPostedMsg:
		return m.commentPosted(msg)
	case commentErrorMsg:
		return m.commentFailed(msg), nil
	case stateChangedMsg:
		return m.stateChanged(msg)
	case stateErrorMsg:
		return m.stateFailed(msg), nil
	case pickerCandidatesMsg:
		return m.candidatesArrived(msg), nil
	case pickerAppliedMsg:
		return m.pickApplied(msg)
	case pickErrorMsg:
		return m.pickFailed(msg), nil
	case reviewContextMsg:
		return m.reviewContextArrived(msg), nil
	case reviewContextErrMsg:
		return m.reviewContextFailed(msg), nil
	case review.CancelledMsg:
		return m.submitCancelled(), nil
	case review.SubmittedMsg:
		return m.submitDone()
	case review.ErrorMsg:
		return m.submitFailed(msg)
	case merge.CancelledMsg:
		return m.mergeCancelled(), nil
	case merge.MergedMsg:
		return m.mergeDone(msg)
	case merge.ErrorMsg:
		return m.mergeFailed(msg)
	case errMsg:
		return m.fetchFailed(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		return m.wheel(msg)
	}
	// The merge popup fetches for itself, and its answer is a type this view
	// cannot name. Everything left over while it is open is its: sizes, keys
	// and its three public messages have already returned above.
	if m.mode == modeMerge {
		var cmd tea.Cmd
		m.merge, cmd = m.merge.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	m.body.SetWidth(msg.Width)
	m.body.SetHeight(max(msg.Height-4, 5))
	m.textarea.SetWidth(msg.Width)
	m.textarea.SetHeight(max(msg.Height-6, 3))
	if m.submit.Active() {
		m.submit, _ = m.submit.Update(msg)
	}
	if m.merge.Active() {
		m.merge, _ = m.merge.Update(msg)
	}
	return m
}

func (m Model) tick(msg spinner.TickMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) itemArrived(msg itemMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	it := msg.item
	m.phase = phaseIdle
	m.state = it.State
	m.errText = ""
	m.declined = ""
	m.labels = labelNames(it.Labels)
	m.assignees = authorLogins(it.Assignees)
	m.url = it.URL
	if it.Kind == gh.ItemPR {
		m.title = i18n.Tf("detail.pr_title", map[string]any{"Number": it.Number, "Title": it.Title})
		m.setContent(prMarkdown(*it.PR))
	} else {
		m.title = i18n.Tf("detail.issue_title", map[string]any{"Number": it.Number, "Title": it.Title})
		m.setContent(issueMarkdown(it))
	}
	return m
}

func (m Model) commentPosted(msg commentPostedMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.errText = ""
	m.textarea.Reset()
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) commentFailed(msg commentErrorMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	m.phase = phaseIdle
	m.errText = msg.err.Error()
	return m
}

func (m Model) stateChanged(msg stateChangedMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.errText = ""
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) stateFailed(msg stateErrorMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = msg.err.Error()
	return m
}

func (m Model) candidatesArrived(msg pickerCandidatesMsg) Model {
	if msg.ref != m.ref {
		return m // an answer for an item the user has already left
	}
	if msg.kind == pickLabels {
		names := make([]string, len(msg.labels))
		colors := make(map[string]string, len(msg.labels))
		for i, l := range msg.labels {
			names[i] = l.Name
			colors[l.Name] = l.Color
		}
		m.picker = newPicker(pickLabels, i18n.T("picker.labels"), names, colors, m.labels)
	} else {
		m.picker = newPicker(pickAssignees, i18n.T("picker.assignees"), msg.users, nil, m.assignees)
	}
	m.mode, m.phase = modePick, phaseIdle
	return m
}

func (m Model) pickApplied(msg pickerAppliedMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) pickFailed(msg pickErrorMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	if m.phase == phaseWorking { // the apply failed; the picker stays up
		m.phase = phaseIdle
	} else { // the candidates never arrived; there is no picker to show
		m.mode, m.phase = modeView, phaseIdle
	}
	m.errText = msg.err.Error()
	return m
}

func (m Model) reviewContextArrived(msg reviewContextMsg) Model {
	if msg.ref != m.ref {
		return m // an answer for an item the user has already left
	}
	target := review.Target{
		PullRequestID:   msg.ctx.PullRequestID,
		PendingID:       msg.ctx.PendingID,
		PendingComments: msg.ctx.PendingCount(),
	}
	m.submit = review.New(m.src, target)
	m.submit, _ = m.submit.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	m.mode, m.phase = modeSubmit, phaseIdle
	m.errText = ""
	return m
}

func (m Model) reviewContextFailed(msg reviewContextErrMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = msg.err.Error()
	return m
}

func (m Model) submitCancelled() Model {
	if m.mode != modeSubmit {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = ""
	return m
}

func (m Model) submitDone() (Model, tea.Cmd) {
	if m.mode != modeSubmit {
		return m, nil
	}
	m.errText = ""
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) submitFailed(msg review.ErrorMsg) (Model, tea.Cmd) {
	if m.mode != modeSubmit {
		return m, nil
	}
	var cmd tea.Cmd
	m.submit, cmd = m.submit.Update(msg)
	m.errText = msg.Err.Error()
	return m, cmd
}

func (m Model) mergeCancelled() Model {
	if m.mode != modeMerge {
		return m
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = ""
	// The zero popup is the inactive one: without this Active() would keep
	// handing it window sizes after it is gone.
	m.merge = merge.Model{}
	return m
}

// mergeDone leaves the view when the pull request was merged: a merged pull
// request is not something to keep reading. Joining or leaving the auto-merge
// queue leaves it open, so the popup keeps the screen and refetches instead.
// The board and the Repos list are refreshed by the root either way, which
// sees the same merge message this one came from, so nothing is sent on.
func (m Model) mergeDone(msg merge.MergedMsg) (Model, tea.Cmd) {
	if m.mode != modeMerge {
		return m, nil
	}
	if !msg.Merged {
		var cmd tea.Cmd
		m.merge, cmd = m.merge.Update(msg)
		return m, cmd
	}
	m.mode, m.phase = modeView, phaseIdle
	m.errText = ""
	m.merge = merge.Model{}
	return m, func() tea.Msg { return ClosedMsg{} }
}

// mergeFailed shows the failure the popup on screen raised. A popup that was
// closed and opened again leaves its own request in flight, and that one's
// failure must not land in this view's footer.
func (m Model) mergeFailed(msg merge.ErrorMsg) (Model, tea.Cmd) {
	if m.mode != modeMerge || !m.merge.Owns(msg) {
		return m, nil
	}
	var cmd tea.Cmd
	m.merge, cmd = m.merge.Update(msg)
	m.errText = msg.Err.Error()
	return m, cmd
}

func (m Model) fetchFailed(msg errMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.phase = phaseIdle
	err := msg.err
	return m, func() tea.Msg { return ErrorMsg{err} }
}

func (m Model) wheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	// The body is drawn only with nothing over it and nothing in flight.
	if m.mode != modeView || m.phase != phaseIdle {
		return m, nil
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

// stillLoading is what the keys that need the item answer with while it is
// on its way. Against the real API the item was still not there four seconds
// after it was asked for, which is long enough for a key that does nothing
// to read as a key that is broken.
func (m Model) stillLoading() Model {
	m.declined = i18n.T("detail.decline_loading")
	return m
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// An overlay's keys have nothing to act on until its fetch answers.
	// modeView is the exception: the body is drawn, and q still leaves.
	if m.mode != modeView && m.phase == phaseLoading {
		return m, nil
	}
	switch m.mode {
	case modeCompose:
		return m.handleComposeKey(msg)
	case modeConfirm:
		return m.handleConfirmKey(msg)
	case modePick:
		return m.handlePickerKey(msg)
	case modeSubmit:
		return m.handleSubmitKey(msg)
	case modeMerge:
		return m.handleMergeKey(msg)
	case modeView:
	}
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "o":
		if m.url == "" {
			return m, nil
		}
		return m, openWeb(m.src, m.ref, m.url)
	case "d":
		// An issue has no diff.
		if m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		ref := m.ref
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
	case "s":
		// An issue has no checks.
		if m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		ref := m.ref
		return m, func() tea.Msg { return OpenChecksMsg{Ref: ref} }
	case "m":
		// An issue has nothing to merge.
		if m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		// Nor has a pull request that is already merged or closed. GitHub
		// answers UNKNOWN for a merged one, so the popup would say it is
		// still working the answer out, for ever.
		if !m.canMerge() {
			return m, nil
		}
		m.mode, m.phase = modeMerge, phaseIdle
		m.errText = ""
		m.merge = merge.New(m.src, m.ref)
		m.merge, _ = m.merge.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.merge.Init()
	case "r":
		m.phase = phaseLoading
		m.declined = ""
		return m, fetch(m.src, m.ref)
	case "c":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		m.mode, m.phase = modeCompose, phaseIdle
		m.errText = ""
		m.textarea.Reset()
		m.textarea.Focus()
		return m, textarea.Blink
	case "x":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		if _, ok := m.stateAction(); !ok {
			return m, nil // merged and the like: no action
		}
		m.mode, m.phase = modeConfirm, phaseIdle
		m.errText = ""
		return m, nil
	case "v":
		// An issue has no review. Unlike the diff view's v, this always
		// fetches first: detail holds no review context of its own.
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		if m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		m.mode, m.phase = modeSubmit, phaseLoading
		m.errText = ""
		return m, fetchReviewContext(m.src, m.ref)
	case "l":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		m.mode, m.phase = modePick, phaseLoading
		m.errText = ""
		return m, fetchLabelPicker(m.src, m.ref)
	case "a":
		if m.phase == phaseLoading {
			return m.stillLoading(), nil
		}
		m.mode, m.phase = modePick, phaseLoading
		m.errText = ""
		return m, fetchAssigneePicker(m.src, m.ref)
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

func (m Model) handlePickerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.phase == phaseWorking {
		return m, nil // ignore every other key while the edit is in flight
	}
	switch msg.String() {
	case "esc":
		m.mode, m.phase = modeView, phaseIdle
		m.errText = ""
		return m, nil
	case "j", "down":
		m.picker.moveDown(visibleRows(m.height))
		return m, nil
	case "k", "up":
		m.picker.moveUp()
		return m, nil
	case " ", "space":
		m.picker.toggle()
		return m, nil
	case "enter":
		add, remove := m.picker.diff()
		if len(add) == 0 && len(remove) == 0 {
			m.mode, m.phase = modeView, phaseIdle // nothing changed: just close
			m.errText = ""
			return m, nil
		}
		m.phase = phaseWorking
		m.errText = ""
		return m, applyPicker(m.src, m.ref, m.picker.kind, add, remove)
	}
	return m, nil
}

func (m Model) handleSubmitKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.submit, cmd = m.submit.Update(msg)
	return m, cmd
}

func (m Model) handleMergeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.merge, cmd = m.merge.Update(msg)
	return m, cmd
}

func (m Model) handleConfirmKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.phase == phaseWorking {
		return m, nil // ignore every other key while the change is in flight
	}
	switch msg.String() {
	case "y":
		closing, ok := m.stateAction()
		if !ok {
			m.mode, m.phase = modeView, phaseIdle
			return m, nil
		}
		m.phase = phaseWorking
		m.errText = ""
		return m, setState(m.src, m.ref, closing)
	case "n", "esc":
		m.mode, m.phase = modeView, phaseIdle
		m.errText = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleComposeKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.phase == phaseWorking {
		return m, nil // ignore every other key while the comment is in flight
	}
	switch msg.String() {
	case "esc":
		m.mode, m.phase = modeView, phaseIdle
		m.errText = ""
		m.textarea.Reset()
		return m, nil
	case "ctrl+s":
		if strings.TrimSpace(m.textarea.Value()) == "" {
			return m, nil // an empty body is not sent
		}
		m.phase = phaseWorking
		m.errText = ""
		return m, postComment(m.src, m.ref, m.textarea.Value())
	}
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

func (m *Model) setContent(md string) {
	width := m.width
	if width <= 0 {
		width = 80
	}
	content := md
	if r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width-2)); err == nil {
		if out, err := r.Render(md); err == nil {
			content = out
		}
	}
	m.body.SetContent(content)
	m.body.GotoTop()
}

func labelNames(labels []gh.Label) []string {
	names := make([]string, len(labels))
	for i, l := range labels {
		names[i] = l.Name
	}
	return names
}

func authorLogins(authors []gh.Author) []string {
	logins := make([]string, len(authors))
	for i, a := range authors {
		logins[i] = a.Login
	}
	return logins
}
