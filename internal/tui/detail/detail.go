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

// itemSource is what the detail view does to the item it shows. A pull
// request and an issue take different GitHub calls for every one of these;
// choosing between them is internal/usecase's job, which is why there is one
// method per operation rather than two.
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

// reviewOpener is what v needs before the review popup can open: the pull
// request's node id and the unsubmitted review, if any. detail has no diff of
// its own, so unlike diff's Source this is the only thing it reads off the
// review context.
type reviewOpener interface {
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
}

// Source is what the detail view needs. repo is "owner/repo"; the empty
// string targets the workspace repository. review.Source is embedded rather
// than repeated: the submission popup this view holds declares exactly what
// it needs.
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

type Model struct {
	src Source
	ref gh.ItemRef

	width, height int

	loading bool
	spin    spinner.Model
	body    viewport.Model
	title   string
	state   gh.ItemState
	url     string

	textarea  textarea.Model
	composing bool
	posting   bool
	postErr   string

	confirming bool
	working    bool
	actionErr  string

	picking       bool
	pickerLoading bool
	applying      bool
	picker        picker
	labels        []string
	assignees     []string

	// submit is the review submission popup (v), a small window drawn over
	// this view rather than a view of its own. openingReview is set between
	// pressing v and the review context it needs arriving; submitErr is a
	// failed submission's text, kept here rather than in submit itself so
	// the popup's own fields stay just its event and its note.
	submit        review.Model
	submitting    bool
	openingReview bool
	submitErr     string
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
		loading:  true,
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
	if m.loading {
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
		m.width, m.height = msg.Width, msg.Height
		m.body.SetWidth(msg.Width)
		m.body.SetHeight(max(msg.Height-4, 5))
		m.textarea.SetWidth(msg.Width)
		m.textarea.SetHeight(max(msg.Height-6, 3))
		if m.submit.Active() {
			m.submit, _ = m.submit.Update(msg)
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case itemMsg:
		if msg.ref != m.ref {
			return m, nil // an answer for an item the user has already left
		}
		it := msg.item
		m.loading = false
		m.state = it.State
		m.actionErr = ""
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
		return m, nil
	case commentPostedMsg:
		m.composing = false
		m.posting = false
		m.postErr = ""
		m.textarea.Reset()
		m.loading = true
		return m, fetch(m.src, m.ref)
	case commentErrorMsg:
		m.posting = false
		m.postErr = msg.err.Error()
		return m, nil
	case stateChangedMsg:
		m.confirming = false
		m.working = false
		m.actionErr = ""
		m.loading = true
		return m, fetch(m.src, m.ref)
	case stateErrorMsg:
		m.confirming = false
		m.working = false
		m.actionErr = msg.err.Error()
		return m, nil
	case pickerCandidatesMsg:
		m.pickerLoading = false
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
		m.picking = true
		return m, nil
	case pickerAppliedMsg:
		m.picking = false
		m.applying = false
		m.loading = true
		return m, fetch(m.src, m.ref)
	case pickErrorMsg:
		if m.picking {
			m.applying = false
			m.picker.err = msg.err.Error()
		} else {
			m.pickerLoading = false
			m.actionErr = msg.err.Error()
		}
		return m, nil
	case reviewContextMsg:
		if msg.ref != m.ref {
			return m, nil // an answer for an item the user has already left
		}
		m.openingReview = false
		target := review.Target{
			PullRequestID:   msg.ctx.PullRequestID,
			PendingID:       msg.ctx.PendingID,
			PendingComments: msg.ctx.PendingCount(),
		}
		m.submit = review.New(m.src, target)
		m.submit, _ = m.submit.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.submitting = true
		m.submitErr = ""
		return m, nil
	case reviewContextErrMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.openingReview = false
		m.actionErr = msg.err.Error()
		return m, nil
	case review.CancelledMsg:
		if !m.submitting {
			return m, nil
		}
		m.submitting = false
		return m, nil
	case review.SubmittedMsg:
		if !m.submitting {
			return m, nil
		}
		m.submitting = false
		m.submitErr = ""
		m.loading = true
		return m, fetch(m.src, m.ref)
	case review.ErrorMsg:
		if !m.submitting {
			return m, nil
		}
		var cmd tea.Cmd
		m.submit, cmd = m.submit.Update(msg)
		m.submitErr = msg.Err.Error()
		return m, cmd
	case errMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.loading = false
		err := msg.err
		return m, func() tea.Msg { return ErrorMsg{err} }
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		// The body is the only thing here that scrolls. The composer, the
		// confirmation, the picker and the submit popup are drawn over it,
		// and a wheel that moved the text underneath them would be
		// scrolling what nobody can see.
		if m.composing || m.confirming || m.picking || m.loading || m.submitting {
			return m, nil
		}
		var cmd tea.Cmd
		m.body, cmd = m.body.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.composing {
		return m.handleComposeKey(msg)
	}
	if m.confirming {
		return m.handleConfirmKey(msg)
	}
	if m.picking {
		return m.handlePickerKey(msg)
	}
	if m.submitting {
		return m.handleSubmitKey(msg)
	}
	if m.pickerLoading || m.openingReview {
		return m, nil // ignore keys while the candidates/review are being fetched
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
		m.loading = true
		return m, fetch(m.src, m.ref)
	case "c":
		if m.loading {
			return m, nil
		}
		m.composing = true
		m.postErr = ""
		m.textarea.Reset()
		m.textarea.Focus()
		return m, textarea.Blink
	case "x":
		if m.loading {
			return m, nil
		}
		if _, ok := m.stateAction(); !ok {
			return m, nil // merged and the like: no action
		}
		m.confirming = true
		m.actionErr = ""
		return m, nil
	case "v":
		// An issue has no review. detail has no diff of its own, so unlike
		// the diff view's v this always needs a fetch first -- there is no
		// review context already sitting on the model to open the popup
		// against.
		if m.loading || m.ref.Kind != gh.ItemPR {
			return m, nil
		}
		m.openingReview = true
		m.actionErr = ""
		return m, fetchReviewContext(m.src, m.ref)
	case "l":
		if m.loading {
			return m, nil
		}
		m.pickerLoading = true
		m.actionErr = ""
		return m, fetchLabelPicker(m.src, m.ref)
	case "a":
		if m.loading {
			return m, nil
		}
		m.pickerLoading = true
		m.actionErr = ""
		return m, fetchAssigneePicker(m.src, m.ref)
	}
	var cmd tea.Cmd
	m.body, cmd = m.body.Update(msg) // j/k and friends scroll the viewport
	return m, cmd
}

func (m Model) handlePickerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.applying {
		return m, nil // ignore every other key while the edit is in flight
	}
	switch msg.String() {
	case "esc":
		m.picking = false
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
			m.picking = false // nothing changed: just close
			return m, nil
		}
		m.applying = true
		m.picker.err = ""
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
	if m.working {
		return m, nil // ignore every other key while the change is in flight
	}
	switch msg.String() {
	case "y":
		closing, ok := m.stateAction()
		if !ok {
			m.confirming = false
			return m, nil
		}
		m.working = true
		m.actionErr = ""
		return m, setState(m.src, m.ref, closing)
	case "n", "esc":
		m.confirming = false
		m.actionErr = ""
		return m, nil
	}
	return m, nil
}

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
		if strings.TrimSpace(m.textarea.Value()) == "" {
			return m, nil // an empty body is not sent
		}
		m.posting = true
		m.postErr = ""
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
