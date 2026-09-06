// Package diff shows one pull request's diff: the files it touches down the
// left, the changes to the selected one on the right, and the review threads
// that hang off its lines.
package diff

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/review"
)

// Source is what the diff view needs from the GitHub layer. repo is
// "owner/repo"; the empty string targets the workspace repository.
//
// review.Source is embedded rather than repeated: the submission popup this
// view holds needs exactly those two methods, and review already declares
// them for exactly this purpose.
type Source interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
	StartReview(pullRequestID string) (string, error)
	AddReviewThread(reviewID string, c gh.PendingComment) error
	DiscardReview(reviewID string) error
	review.Source
}

// ClosedMsg tells the parent the user left the diff view.
type ClosedMsg struct{}

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type diffMsg struct {
	ref   gh.ItemRef
	files []gh.FileDiff
}

type reviewMsg struct {
	ref gh.ItemRef
	ctx gh.ReviewContext
}

// errMsg is the diff fetch's own failure: with no diff there is nothing to
// show, so it escalates to the parent's whole-screen error view (ErrorMsg).
type errMsg struct {
	ref gh.ItemRef
	err error
}

// reviewErrMsg is the review context's own failure: the diff itself may
// still be readable, so this is shown on one line above the key bar instead
// of escalating (.claude/rules/errors.md's Bubble Tea section).
type reviewErrMsg struct {
	ref gh.ItemRef
	err error
}

// rowKind separates the three things a line of the diff pane can be.
type rowKind int

const (
	rowHunkHeader rowKind = iota
	rowLine
	rowNote      // "binary file", "no files changed": text with nothing behind it
	rowThread    // one comment of an open review thread
	rowCollapsed // a count of settled (resolved or outdated) threads
)

// row is one drawable line of the diff pane. hunk is the index of the hunk it
// belongs to, which is what { and } move between; a note belongs to none and
// carries -1. thread, comment and key are set on rowThread and rowCollapsed
// rows: key names the thread's position (see threadKey) and is what enter
// looks up in Model.expanded.
type row struct {
	kind    rowKind
	hunk    int
	line    gh.DiffLine
	text    string
	thread  gh.ReviewThread
	comment gh.ThreadComment
	key     string
}

type Model struct {
	src Source
	ref gh.ItemRef

	width, height int

	loading bool
	spin    spinner.Model

	files   []gh.FileDiff
	file    int
	fileTop int // the first file drawn in the sidebar

	// review is the review context: the header's title and branches, and the
	// threads already on the diff. It arrives separately from files (fetch),
	// and either may land first.
	review gh.ReviewContext

	// reviewErr is set when the review context fails to fetch. The diff may
	// still be readable, so this is drawn on its own footer line rather than
	// replacing the whole screen; nil means no failure to show.
	reviewErr error

	// expanded is the set of settled threads the user has opened, keyed by
	// threadKey. It survives a refetch because it is keyed by position, not
	// by the thread's own id.
	expanded map[string]bool

	// rows is the current file, flattened for drawing. It is rebuilt whenever
	// the file changes rather than on every draw, because View may do no work
	// that a state change did not ask for.
	rows []row
	row  int
	top  int // the first row on screen

	// sidebar is where the cursor is: false in the diff pane, true in the
	// file list. h and l move between them.
	sidebar bool

	// textarea, composing and posting are the line-comment composer. It is
	// the same shape as detail's: ctrl+s sends, esc discards the draft.
	textarea  textarea.Model
	composing bool
	posting   bool
	postErr   string

	// target is the line and side the open (or in-flight) comment was
	// started against, captured by startComposing at c-time rather than read
	// again from the cursor at send time (see comment.go).
	target gh.PendingComment

	// submit is the review submission popup (v), a small window drawn over
	// this view rather than a view of its own (see review.go). submitErr is
	// a failed submission's text, kept here rather than in submit itself so
	// the popup's own fields stay just its event and its note.
	submit     review.Model
	submitting bool
	submitErr  string

	// discarding asks before X throws the pending review away.
	// discardWorking is separate from discarding so a second y sent before
	// DiscardReview's answer lands cannot fire the call twice.
	discarding     bool
	discardWorking bool
	discardErr     string
}

// New builds the view for one pull request. It takes only what names the
// pull request: the title, the branches and the size of the change arrive
// with the review context (Task 7), because a Work card and a Repos row know
// different amounts about a pull request and neither knows all of it. It also
// keeps the argument list to two (.claude/rules/go-style.md).
func New(src Source, ref gh.ItemRef) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	ta := textarea.New()
	ta.Placeholder = i18n.T("diff.comment_placeholder")
	ta.ShowLineNumbers = false
	ta.SetHeight(composerRows)
	return Model{src: src, ref: ref, loading: true, spin: s, textarea: ta}
}

// Init starts the fetch. Unlike the Work board this view is built once per
// pull request, so there is no refresh whose cancel function has to outlive
// an Init with a value receiver.
func (m Model) Init() tea.Cmd { return tea.Batch(m.spin.Tick, m.fetch()) }

// fetch takes the diff and the review context in parallel: neither has to
// wait for the other, and the view draws with whatever has landed.
func (m Model) fetch() tea.Cmd {
	return tea.Batch(m.fetchDiff(), m.fetchReview())
}

func (m Model) fetchDiff() tea.Cmd {
	src, ref := m.src, m.ref
	return func() tea.Msg {
		files, err := src.PRDiff(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return errMsg{ref: ref, err: err}
		}
		return diffMsg{ref: ref, files: files}
	}
}

func (m Model) fetchReview() tea.Cmd {
	src, ref := m.src, m.ref
	return func() tea.Msg {
		ctx, err := src.PRReviewContext(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return reviewErrMsg{ref: ref, err: err}
		}
		return reviewMsg{ref: ref, ctx: ctx}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// Below minWidthForSidebar the file list is not drawn at all; a
		// cursor left pointing at it would be on a pane that no longer
		// exists.
		if !m.showSidebar() {
			m.sidebar = false
		}
		m.textarea.SetWidth(max(m.width, 0))
		if m.submit.Active() {
			m.submit, _ = m.submit.Update(msg)
		}
		return m, nil
	case diffMsg:
		// The request for the pull request the user just left is still in
		// flight; its answer must not replace this one's.
		if msg.ref != m.ref {
			return m, nil
		}
		m.loading = false
		m.files = msg.files
		m.file, m.row, m.top, m.fileTop = 0, 0, 0, 0
		m.rows = m.buildRows()
		return m, nil
	case reviewMsg:
		// The review context for the pull request the user just left is
		// still in flight; its answer must not land here.
		if msg.ref != m.ref {
			return m, nil
		}
		m.review = msg.ctx
		m.reviewErr = nil
		m.rows = m.buildRows()
		m.row = clamp(m.row, len(m.rows)-1)
		m = m.follow()
		return m, nil
	case reviewErrMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.reviewErr = msg.err
		return m, nil
	case errMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.loading = false
		return m, func() tea.Msg { return ErrorMsg{Err: msg.err} }
	case commentPostedMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.posting = false
		m.postErr = ""
		m.textarea.Reset()
		m.target = gh.PendingComment{}
		m.review.PendingID = msg.reviewID
		return m, m.fetchReview()
	case commentErrorMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.posting = false
		m.composing = true
		m.postErr = msg.err.Error()
		return m, nil
	case review.CancelledMsg:
		// Also reaches here when the diff is drawn over the detail view and
		// the submission was detail's own (broadcast hands the message to
		// both); only the one that actually opened the popup acts on it.
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
		m.review.PendingID = ""
		return m, m.fetchReview()
	case review.ErrorMsg:
		if !m.submitting {
			return m, nil
		}
		var cmd tea.Cmd
		m.submit, cmd = m.submit.Update(msg)
		m.submitErr = msg.Err.Error()
		return m, cmd
	case discardedMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.discardWorking = false
		if msg.err != nil {
			m.discardErr = msg.err.Error()
			return m, nil
		}
		m.discarding = false
		m.review.PendingID = ""
		return m, m.fetchReview()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.composing || m.posting {
		return m.handleComposeKey(msg)
	}
	if m.submitting {
		return m.handleSubmitKey(msg)
	}
	if m.discarding {
		return m.handleDiscardKey(msg)
	}
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "r":
		m.loading = true
		m.reviewErr = nil
		return m, tea.Batch(m.spin.Tick, m.fetch())
	case "j", "down":
		return m.moveRow(1), nil
	case "k", "up":
		return m.moveRow(-1), nil
	case "]":
		return m.moveFile(1), nil
	case "[":
		return m.moveFile(-1), nil
	case "}":
		return m.moveHunk(1), nil
	case "{":
		return m.moveHunk(-1), nil
	case "h":
		// There is nowhere to move to once the sidebar is folded.
		if m.showSidebar() {
			m.sidebar = true
		}
		return m, nil
	case "l":
		m.sidebar = false
		return m, nil
	case "enter":
		return m.toggleCollapsed(), nil
	case "c":
		return m.startComposing(), nil
	case "v":
		return m.openSubmit(), nil
	case "X":
		return m.startDiscard(), nil
	}
	return m, nil
}

// toggleCollapsed opens or closes the settled thread under the cursor.
// Opening replaces the rowCollapsed row with the thread's own rows, which
// moves the cursor onto one of them (same index, new content) rather than
// off the thread entirely, so a second enter must still recognise it as the
// same thread to close it again.
//
// This is why rowThread is accepted here too, not only rowCollapsed: once
// open, an every-comment thread draws one rowThread per comment, all sharing
// the same key, and enter has to close the group from whichever of those
// rows the cursor happens to be on -- not only the first.
func (m Model) toggleCollapsed() Model {
	r := m.currentRow()
	if r.kind != rowCollapsed && r.kind != rowThread {
		return m
	}
	if r.key == "" {
		return m
	}
	if m.expanded == nil {
		m.expanded = map[string]bool{}
	}
	m.expanded[r.key] = !m.expanded[r.key]
	m.rows = m.buildRows()
	m.row = clamp(m.row, len(m.rows)-1)
	return m.follow()
}

// buildRows flattens the selected file into the lines the diff pane draws.
func (m Model) buildRows() []row {
	if len(m.files) == 0 {
		return []row{{kind: rowNote, hunk: -1, text: i18n.T("diff.no_changes")}}
	}
	f := m.files[m.file]
	if f.Binary {
		return []row{{kind: rowNote, hunk: -1, text: i18n.T("diff.binary")}}
	}
	var rows []row
	placed := map[string]bool{}
	for i, h := range f.Hunks {
		rows = append(rows, row{kind: rowHunkHeader, hunk: i, text: h.Header})
		for _, l := range h.Lines {
			// A literal tab has no fixed display width: a terminal advances
			// to the next tab stop, and lipgloss's own Style.Render silently
			// expands it to four spaces while ansi.StringWidth counts it as
			// zero. Expanding it here, once, before anything measures or
			// truncates the line, is what keeps every later width
			// calculation honest.
			l.Text = expandTabs(l.Text)
			rows = append(rows, row{kind: rowLine, hunk: i, line: l})
			line, side := l.Line()
			if tr := m.threadRows(i, f.Path, line, side); tr != nil {
				placed[threadKey(f.Path, line, side)] = true
				rows = append(rows, tr...)
			}
		}
	}
	rows = append(rows, m.orphanRows(placed)...)
	return rows
}

func (m Model) moveRow(delta int) Model {
	if m.sidebar {
		return m.moveFile(delta)
	}
	m.row = clamp(m.row+delta, len(m.rows)-1)
	return m.follow()
}

func (m Model) moveFile(delta int) Model {
	if len(m.files) == 0 {
		return m
	}
	m.file = clamp(m.file+delta, len(m.files)-1)
	m.row, m.top = 0, 0
	m.rows = m.buildRows()
	return m.followSidebar()
}

// followSidebar scrolls the file list so the selected file stays visible,
// the same way follow keeps the diff pane's cursor on screen. Each file
// takes two lines (its path and its size), so the window is counted in
// files, not lines.
func (m Model) followSidebar() Model {
	visible := max(m.paneHeight()/2, 1)
	if m.file < m.fileTop {
		m.fileTop = m.file
	}
	if m.file >= m.fileTop+visible {
		m.fileTop = m.file - visible + 1
	}
	m.fileTop = max(m.fileTop, 0)
	return m
}

// moveHunk puts the cursor on the header of the next or previous hunk. It
// moves between hunks rather than by a fixed number of lines, which is the
// whole point of the key: a hunk is as long as it is.
func (m Model) moveHunk(delta int) Model {
	if len(m.rows) == 0 {
		return m
	}
	want := m.rows[m.row].hunk + delta
	for i, r := range m.rows {
		if r.kind == rowHunkHeader && r.hunk == want {
			m.row = i
			return m.follow()
		}
	}
	return m
}

// follow scrolls the window so the cursor stays on it. Only the pane the
// cursor is in scrolls, the same rule the Work board follows.
func (m Model) follow() Model {
	h := m.paneHeight()
	if m.row < m.top {
		m.top = m.row
	}
	if m.row >= m.top+h {
		m.top = m.row - h + 1
	}
	m.top = max(m.top, 0)
	return m
}

// currentRow returns the row under the cursor, and the zero row when there
// are none: a key can be pressed before any diff has landed.
func (m Model) currentRow() row {
	if len(m.rows) == 0 {
		return row{}
	}
	return m.rows[m.row]
}

// clamp keeps v within [0, hi]. Every caller clamps a cursor index, which is
// never negative, so the floor is fixed rather than a parameter.
func clamp(v, hi int) int {
	if hi < 0 {
		return 0
	}
	return min(max(v, 0), hi)
}
