// Package merge is the popup that merges a pull request: which method the
// commits land by, whether to wait for the checks, and the one enter that
// sends it.
//
// It is a popup rather than a view of its own, so it has no place in the
// root model's stack. The detail view holds one and draws it over itself.
// Unlike internal/tui/review it fetches for itself: r has to be able to ask
// GitHub again while the popup stays open (standalone design §4.4.4).
package merge

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// Source is what merging needs.
type Source interface {
	PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error)
	MergePR(pullRequestID string, method gh.MergeMethod) error
	EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error
	DisableAutoMerge(pullRequestID string) error
}

// MergedMsg tells the holder the pull request was merged, queued, or taken
// out of the queue: whichever it was, what is on screen is now stale.
type MergedMsg struct{}

// CancelledMsg tells the holder to take the popup away.
type CancelledMsg struct{}

// ErrorMsg carries a failure the holder shows at footer level, the same way
// internal/tui/review hands its failures up (.claude/rules/errors.md).
type ErrorMsg struct{ Err error }

// contextMsg carries the fetch's answer for the pull request it was sent
// for; an answer for another one is dropped, the way the checks view drops
// a log that arrives after the user has moved on.
type contextMsg struct {
	ref gh.ItemRef
	ctx gh.MergeContext
}

type Model struct {
	src Source
	ref gh.ItemRef

	ctx     gh.MergeContext
	loading bool
	row     int
	auto    bool
	sending bool

	width, height int
}

// New builds the popup. Init is what starts the fetch.
func New(src Source, ref gh.ItemRef) Model {
	return Model{src: src, ref: ref, loading: true}
}

// Active reports whether the popup has anything to show. A zero Model,
// before New has built it, reports false.
func (m Model) Active() bool { return m.ref.Number != 0 }

func (m Model) Init() tea.Cmd { return fetch(m.src, m.ref) }

func fetch(src Source, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		c, err := src.PRMergeContext(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return ErrorMsg{Err: err}
		}
		return contextMsg{ref: ref, ctx: c}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case contextMsg:
		if msg.ref != m.ref {
			return m, nil
		}
		m.ctx = msg.ctx
		m.loading = false
		m.row = 0
		// auto is what enter would turn on. On a pull request that already
		// has an auto-merge, enter cancels it instead and auto is unused
		// (standalone design §4.4.4).
		m.auto = false
		return m, nil
	case ErrorMsg:
		m.loading = false
		m.sending = false
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.sending {
		return m, nil // ignore every other key while the merge is in flight
	}
	switch msg.String() {
	case "esc":
		return m, func() tea.Msg { return CancelledMsg{} }
	case "r":
		m.loading = true
		return m, fetch(m.src, m.ref)
	}
	if m.loading {
		return m, nil
	}
	switch msg.String() {
	case "j", "down":
		if m.row < len(m.ctx.Methods)-1 {
			m.row++
		}
		return m, nil
	case "k", "up":
		if m.row > 0 {
			m.row--
		}
		return m, nil
	case " ", "space":
		if m.ctx.CanAutoMerge() && !m.ctx.AutoMergeEnabled {
			m.auto = !m.auto
		}
		return m, nil
	case "enter":
		return m.send()
	}
	return m, nil
}

// send does the one thing the popup is for. Leaving the auto-merge queue is
// the one action offered while merging itself is blocked: a pull request
// that cannot be merged is no reason to be stuck with a queued merge.
func (m Model) send() (Model, tea.Cmd) {
	switch {
	case m.ctx.AutoMergeEnabled:
		return m.sendCmd(func() error { return m.src.DisableAutoMerge(m.ctx.PullRequestID) })
	case m.ctx.Block() != gh.BlockNone:
		return m, nil
	case m.auto:
		method := m.method()
		return m.sendCmd(func() error { return m.src.EnableAutoMerge(m.ctx.PullRequestID, method) })
	default:
		method := m.method()
		return m.sendCmd(func() error { return m.src.MergePR(m.ctx.PullRequestID, method) })
	}
}

// method is the one on the cursor. A repository with no method allowed at
// all cannot happen -- GitHub refuses to turn the last one off -- so the
// fallback is only there to keep the index safe.
func (m Model) method() gh.MergeMethod {
	if m.row < len(m.ctx.Methods) {
		return m.ctx.Methods[m.row]
	}
	return gh.MergeSquash
}

func (m Model) sendCmd(do func() error) (Model, tea.Cmd) {
	m.sending = true
	return m, func() tea.Msg {
		if err := do(); err != nil {
			return ErrorMsg{Err: err}
		}
		return MergedMsg{}
	}
}
