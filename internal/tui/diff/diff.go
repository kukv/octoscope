// Package diff shows one pull request's diff: the files it touches down the
// left, the changes to the selected one on the right, and the review threads
// that hang off its lines.
package diff

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// Source is what the diff view needs from the GitHub layer. repo is
// "owner/repo"; the empty string targets the workspace repository.
type Source interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
}

// ClosedMsg tells the parent the user left the diff view.
type ClosedMsg struct{}

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type diffMsg struct {
	ref   gh.ItemRef
	files []gh.FileDiff
}

type errMsg struct {
	ref gh.ItemRef
	err error
}

// rowKind separates the three things a line of the diff pane can be.
type rowKind int

const (
	rowHunkHeader rowKind = iota
	rowLine
	rowNote // "binary file", "no files changed": text with nothing behind it
)

// row is one drawable line of the diff pane. hunk is the index of the hunk it
// belongs to, which is what { and } move between; a note belongs to none and
// carries -1.
type row struct {
	kind rowKind
	hunk int
	line gh.DiffLine
	text string
}

type Model struct {
	src Source
	ref gh.ItemRef

	width, height int

	loading bool
	spin    spinner.Model

	files []gh.FileDiff
	file  int

	// rows is the current file, flattened for drawing. It is rebuilt whenever
	// the file changes rather than on every draw, because View may do no work
	// that a state change did not ask for.
	rows []row
	row  int
	top  int // the first row on screen

	// sidebar is where the cursor is: false in the diff pane, true in the
	// file list. h and l move between them.
	sidebar bool
}

// New builds the view for one pull request. It takes only what names the
// pull request: the title, the branches and the size of the change arrive
// with the review context (Task 7), because a Work card and a Repos row know
// different amounts about a pull request and neither knows all of it. It also
// keeps the argument list to two (.claude/rules/go-style.md).
func New(src Source, ref gh.ItemRef) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{src: src, ref: ref, loading: true, spin: s}
}

// Init starts the fetch. Unlike the Work board this view is built once per
// pull request, so there is no refresh whose cancel function has to outlive
// an Init with a value receiver.
func (m Model) Init() tea.Cmd { return tea.Batch(m.spin.Tick, m.fetch()) }

func (m Model) fetch() tea.Cmd {
	src, ref := m.src, m.ref
	return func() tea.Msg {
		files, err := src.PRDiff(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return errMsg{ref: ref, err: err}
		}
		return diffMsg{ref: ref, files: files}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case diffMsg:
		// The request for the pull request the user just left is still in
		// flight; its answer must not replace this one's.
		if msg.ref != m.ref {
			return m, nil
		}
		m.loading = false
		m.files = msg.files
		m.file, m.row, m.top = 0, 0, 0
		m.rows = m.buildRows()
		return m, nil
	case errMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.loading = false
		return m, func() tea.Msg { return ErrorMsg{Err: msg.err} }
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "r":
		m.loading = true
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
		m.sidebar = true
		return m, nil
	case "l":
		m.sidebar = false
		return m, nil
	}
	return m, nil
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
		}
	}
	return rows
}

func (m Model) moveRow(delta int) Model {
	if m.sidebar {
		return m.moveFile(delta)
	}
	m.row = clamp(m.row+delta, 0, len(m.rows)-1)
	return m.follow()
}

func (m Model) moveFile(delta int) Model {
	if len(m.files) == 0 {
		return m
	}
	m.file = clamp(m.file+delta, 0, len(m.files)-1)
	m.row, m.top = 0, 0
	m.rows = m.buildRows()
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

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}
