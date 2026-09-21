// Package merge is the popup that merges a pull request: which method the
// commits land by, whether to wait for the checks, and the one enter that
// sends it.
//
// It is a popup rather than a view of its own, so it has no place in the
// root model's stack. The detail view holds one and draws it over itself.
// Unlike internal/app/presentation/tui/review it fetches for itself: r has to be able to ask
// GitHub again while the popup stays open (standalone design §4.4.4).
package merge

import (
	"context"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
)

// nextGen numbers every fetch the package sends. Two fetches for the same
// pull request can be in flight -- r starts one while the first is still
// out, and a popup that has been closed and opened again is another pair --
// and only a number that never repeats can tell their answers apart.
var nextGen atomic.Int64

type Source interface {
	PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error)
	MergePR(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error
	EnableAutoMerge(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error
	DisableAutoMerge(ctx context.Context, pr domain.PullRequestHandle) error
}

// MergedMsg tells the holder the pull request was merged, queued, or taken
// out of the queue: whichever it was, what is on screen is now stale.
// Merged is true only for the merge itself; joining or leaving the queue
// leaves the pull request open and worth reading.
type MergedMsg struct{ Merged bool }

type CancelledMsg struct{}

// ErrorMsg carries a failure the holder shows at footer level, the same way
// internal/app/presentation/tui/review hands its failures up (.claude/rules/errors.md). It
// names what raised it so a popup that has moved on can drop it; the holder
// asks Owns before it shows the message.
type ErrorMsg struct {
	Ref domain.ItemRef
	Gen int64
	Err error
}

// contextMsg carries the fetch's answer for the pull request and the fetch
// it was sent for; an older answer is dropped, the way the checks view drops
// a log that arrives after the user has moved on.
type contextMsg struct {
	ref domain.ItemRef
	gen int64
	ctx domain.MergeContext
}

type Model struct {
	src Source
	ref domain.ItemRef
	gen int64

	ctx     domain.MergeContext
	loading bool
	row     int
	auto    bool
	sending bool

	width, height int
}

// New builds the popup. Init is what starts the fetch.
func New(src Source, ref domain.ItemRef) Model {
	return Model{src: src, ref: ref, gen: nextGen.Add(1), loading: true}
}

// Active reports whether the popup has anything to show. A zero Model,
// before New has built it, reports false.
func (m Model) Active() bool { return m.ref.Number != 0 }

// Owns reports whether a failure came from this popup's own fetch or its own
// mutation.
func (m Model) Owns(msg ErrorMsg) bool { return msg.Ref == m.ref && msg.Gen == m.gen }

func (m Model) Init() tea.Cmd { return fetch(m.src, m.ref, m.gen) }

func fetch(src Source, ref domain.ItemRef, gen int64) tea.Cmd {
	return func() tea.Msg {
		c, err := src.PRMergeContext(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return ErrorMsg{Ref: ref, Gen: gen, Err: err}
		}
		return contextMsg{ref: ref, gen: gen, ctx: c}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case contextMsg:
		if msg.ref != m.ref || msg.gen != m.gen {
			return m, nil
		}
		m.ctx = msg.ctx
		m.loading = false
		// What the user chose survives the refetch: r must not silently undo
		// it. Both are re-validated against the answer that just arrived,
		// which may allow fewer methods or no auto-merge at all.
		m.row = min(m.row, max(len(m.ctx.Methods)-1, 0))
		m.auto = m.auto && m.ctx.CanAutoMerge() && !m.ctx.AutoMergeEnabled
		return m, nil
	case MergedMsg:
		if msg.Merged {
			return m, nil
		}
		// The pull request is still open, only its queue changed: ask GitHub
		// what it looks like now rather than showing the state it had before.
		m.sending = false
		return m.refetch()
	case ErrorMsg:
		if !m.Owns(msg) {
			return m, nil
		}
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
		return m.refetch()
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
	case "a":
		return m.mergeAsAdmin()
	case "enter":
		return m.send()
	}
	return m, nil
}

// mergeAsAdmin pushes the merge past what is holding it. It is a key of its
// own rather than a meaning enter takes on when merging is blocked: breaking
// a protection must not share a keystroke with an ordinary merge, or a
// mistake looks exactly like the thing the user does every day.
func (m Model) mergeAsAdmin() (Model, tea.Cmd) {
	if !m.ctx.CanMergeAsAdmin() {
		return m, nil
	}
	method := m.method()
	return m.sendCmd(true, func(ctx context.Context) error { return m.src.MergePR(ctx, m.ctx.PullRequest, method) })
}

// send does the one thing the popup is for. Leaving the auto-merge queue is
// the one action offered while merging itself is blocked: a pull request
// that cannot be merged is no reason to be stuck with a queued merge.
func (m Model) send() (Model, tea.Cmd) {
	switch {
	case m.ctx.AutoMergeEnabled:
		return m.sendCmd(false, func(ctx context.Context) error { return m.src.DisableAutoMerge(ctx, m.ctx.PullRequest) })
	case m.ctx.Block != domain.BlockNone:
		return m, nil
	case m.auto:
		method := m.method()
		return m.sendCmd(false, func(ctx context.Context) error { return m.src.EnableAutoMerge(ctx, m.ctx.PullRequest, method) })
	default:
		method := m.method()
		return m.sendCmd(true, func(ctx context.Context) error { return m.src.MergePR(ctx, m.ctx.PullRequest, method) })
	}
}

// method is the one on the cursor. A repository with no method allowed at
// all cannot happen -- GitHub refuses to turn the last one off -- so the
// fallback is only there to keep the index safe.
func (m Model) method() domain.MergeMethod {
	if m.row < len(m.ctx.Methods) {
		return m.ctx.Methods[m.row]
	}
	return domain.MergeSquash
}

func (m Model) sendCmd(merged bool, do func(ctx context.Context) error) (Model, tea.Cmd) {
	m.sending = true
	ref, gen := m.ref, m.gen
	return m, func() tea.Msg {
		if err := do(context.Background()); err != nil {
			return ErrorMsg{Ref: ref, Gen: gen, Err: err}
		}
		return MergedMsg{Merged: merged}
	}
}

// refetch asks GitHub again under a fresh generation, so the answer to the
// fetch it replaces is dropped when it lands.
func (m Model) refetch() (Model, tea.Cmd) {
	m.loading = true
	m.gen = nextGen.Add(1)
	return m, fetch(m.src, m.ref, m.gen)
}
