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
}

// ClosedMsg tells the parent the user left the detail view.
type ClosedMsg struct{}

// OpenDiffMsg asks the parent to show the diff of the shown pull request.
type OpenDiffMsg struct{ Ref gh.ItemRef }

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type (
	// itemMsg carries a fetch's answer along with the item it is about. The
	// detail view is rebuilt for each item the user opens, but the request
	// for the last one is still running: without the ref, its answer would
	// land here and show the wrong item for as long as the current fetch
	// takes.
	itemMsg struct {
		ref  gh.ItemRef
		item usecase.Item
	}
	errMsg struct {
		ref gh.ItemRef
		err error
	}
	commentPostedMsg    struct{}
	commentErrorMsg     struct{ err error }
	stateChangedMsg     struct{}
	stateErrorMsg       struct{ err error }
	pickerCandidatesMsg struct {
		kind   pickerKind
		labels []gh.Label
		users  []string
	}
	pickerAppliedMsg struct{}
	pickErrorMsg     struct{ err error }

	// reviewContextMsg and reviewContextErrMsg carry v's own fetch: unlike
	// itemMsg, this one is not part of the initial load, so it still needs
	// the ref guard against an item the user has since left.
	reviewContextMsg struct {
		ref gh.ItemRef
		ctx gh.ReviewContext
	}
	reviewContextErrMsg struct {
		ref gh.ItemRef
		err error
	}
)

// mode is which overlay is on screen. Ten parallel bools named 2^10 nominal
// states for the eleven this view reaches, and Update, handleKey and View
// each assumed a different subset of them (.claude/rules/tui.md). mode and
// phase name 5x3, and the eleven are: modeView idle or loading; modeCompose
// and modeConfirm idle or working; modePick loading, idle or working;
// modeSubmit loading or idle -- never working, because review.Model owns the
// send. The other four are unreachable: phaseWorking is only ever set by the
// compose, confirm and picker key handlers, and phaseLoading only alongside
// the mode it is fetching for.
type mode uint8

const (
	modeView    mode = iota // the body on its own
	modeCompose             // the comment composer
	modeConfirm             // the close/reopen confirmation
	modePick                // the label/assignee picker
	modeSubmit              // the review submission popup
)

// phase is where the current mode is in its own round trip.
type phase uint8

const (
	phaseIdle    phase = iota
	phaseLoading       // fetching what the mode needs to open
	phaseWorking       // sending
)

type Model struct {
	src Source
	ref gh.ItemRef

	width, height int

	mode  mode
	phase phase

	// errText is the last failure, whatever produced it: which mode is on
	// screen decides where it is drawn. What one string cannot carry is an
	// error that outlives the mode it came from -- opening an overlay clears
	// the body's error rather than draw someone else's failure inside it.
	errText string

	spin  spinner.Model
	body  viewport.Model
	title string
	state gh.ItemState
	url   string

	textarea textarea.Model

	picker    picker
	labels    []string
	assignees []string

	// submit is the review submission popup (v), a small window drawn over
	// this view rather than a view of its own.
	submit review.Model
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
			return commentErrorMsg{err}
		}
		return commentPostedMsg{}
	}
}

// stateAction reports whether the shown item can change state, and if so
// whether the action is a close (true) or a reopen (false). Nothing has been
// fetched yet is not a state of its own: loading is, and every caller checks
// it first.
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

func setState(src Source, ref gh.ItemRef, closing bool) tea.Cmd {
	return func() tea.Msg {
		if err := src.SetState(ref, closing); err != nil {
			return stateErrorMsg{err}
		}
		return stateChangedMsg{}
	}
}

func fetchLabelPicker(src candidateSource, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		labels, err := src.ListLabels(context.Background(), ref.Repo)
		if err != nil {
			return pickErrorMsg{err}
		}
		return pickerCandidatesMsg{kind: pickLabels, labels: labels}
	}
}

func fetchAssigneePicker(src candidateSource, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		users, err := src.ListAssignees(context.Background(), ref.Repo)
		if err != nil {
			return pickErrorMsg{err}
		}
		return pickerCandidatesMsg{kind: pickAssignees, users: users}
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
			return pickErrorMsg{err}
		}
		return pickerAppliedMsg{}
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
		return m.commentPosted()
	case commentErrorMsg:
		return m.commentFailed(msg.err), nil
	case stateChangedMsg:
		return m.stateChanged()
	case stateErrorMsg:
		return m.stateFailed(msg.err), nil
	case pickerCandidatesMsg:
		return m.candidatesArrived(msg), nil
	case pickerAppliedMsg:
		return m.pickApplied()
	case pickErrorMsg:
		return m.pickFailed(msg.err), nil
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
	case errMsg:
		return m.fetchFailed(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		return m.wheel(msg)
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
	return m
}

func (m Model) tick(msg spinner.TickMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

// itemArrived puts a fetched item on screen. An answer for an item the user
// has already left is dropped: the request for the last one is still running
// while the view is rebuilt for the new one.
func (m Model) itemArrived(msg itemMsg) Model {
	if msg.ref != m.ref {
		return m
	}
	it := msg.item
	m.phase = phaseIdle
	m.state = it.State
	m.errText = ""
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

func (m Model) commentPosted() (Model, tea.Cmd) {
	m.errText = ""
	m.textarea.Reset()
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) commentFailed(err error) Model {
	m.phase = phaseIdle
	m.errText = err.Error()
	return m
}

func (m Model) stateChanged() (Model, tea.Cmd) {
	m.errText = ""
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) stateFailed(err error) Model {
	m.mode, m.phase = modeView, phaseIdle
	m.errText = err.Error()
	return m
}

func (m Model) candidatesArrived(msg pickerCandidatesMsg) Model {
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

func (m Model) pickApplied() (Model, tea.Cmd) {
	m.mode, m.phase = modeView, phaseLoading
	return m, fetch(m.src, m.ref)
}

func (m Model) pickFailed(err error) Model {
	if m.phase == phaseWorking { // the apply failed; the picker stays up
		m.phase = phaseIdle
	} else { // the candidates never arrived; there is no picker to show
		m.mode, m.phase = modeView, phaseIdle
	}
	m.errText = err.Error()
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

func (m Model) fetchFailed(msg errMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.phase = phaseIdle
	err := msg.err
	return m, func() tea.Msg { return ErrorMsg{err} }
}

func (m Model) wheel(msg tea.MouseWheelMsg) (Model, tea.Cmd) {
	// The body is the only thing here that scrolls, and it is only on
	// screen with nothing over it and nothing in flight.
	if m.mode != modeView || m.phase != phaseIdle {
		return m, nil
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	// Nothing an overlay's keys could act on is on screen until what it was
	// opened for arrives. modeView is the exception: the body is drawn, and
	// q still leaves.
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
	case modeView:
	}
	switch msg.String() {
	case "q", "esc":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "o":
		// Before the item lands there is no address to open.
		if m.url == "" {
			return m, nil
		}
		return m, openWeb(m.src, m.ref, m.url)
	case "d":
		// An issue has no diff. Opening an empty diff view would be a worse
		// answer than doing nothing.
		if m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		ref := m.ref
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
	case "r":
		m.phase = phaseLoading
		return m, fetch(m.src, m.ref)
	case "c":
		if m.phase == phaseLoading {
			return m, nil
		}
		m.mode, m.phase = modeCompose, phaseIdle
		m.errText = ""
		m.textarea.Reset()
		m.textarea.Focus()
		return m, textarea.Blink
	case "x":
		if m.phase == phaseLoading {
			return m, nil
		}
		if _, ok := m.stateAction(); !ok {
			return m, nil // merged and the like: no action
		}
		m.mode, m.phase = modeConfirm, phaseIdle
		m.errText = ""
		return m, nil
	case "v":
		// An issue has no review. detail has no diff of its own, so unlike
		// the diff view's v this always needs a fetch first -- there is no
		// review context already sitting on the model to open the popup
		// against.
		if m.phase == phaseLoading || m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		m.mode, m.phase = modeSubmit, phaseLoading
		m.errText = ""
		return m, fetchReviewContext(m.src, m.ref)
	case "l":
		if m.phase == phaseLoading {
			return m, nil
		}
		m.mode, m.phase = modePick, phaseLoading
		m.errText = ""
		return m, fetchLabelPicker(m.src, m.ref)
	case "a":
		if m.phase == phaseLoading {
			return m, nil
		}
		m.mode, m.phase = modePick, phaseLoading
		m.errText = ""
		return m, fetchAssigneePicker(m.src, m.ref)
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg) // j/k and friends scroll the viewport
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

// setContent renders markdown through glamour into the viewport, falling back
// to the raw markdown when glamour cannot render it.
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
