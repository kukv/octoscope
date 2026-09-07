// Package checks shows one pull request's checks: what ran down the left,
// the log of the selected one on the right.
package checks

import (
	"context"
	"sort"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// Source is what the checks view needs. repo is "owner/repo"; the empty
// string targets the workspace repository.
type Source interface {
	PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)
	JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error
	OpenWeb(url string) error
}

// ClosedMsg tells the parent the user left the checks view.
type ClosedMsg struct{}

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type checksMsg struct {
	ref    gh.ItemRef
	checks gh.Checks
}

type errMsg struct {
	ref gh.ItemRef
	err error
}

type Model struct {
	src Source
	ref gh.ItemRef

	width, height int

	loading bool
	spin    spinner.Model

	checks gh.Checks
	// order is the checks as drawn: failing workflows first, and the checks
	// of one workflow together. It is rebuilt when the checks arrive rather
	// than on every draw, because View may do no work of its own.
	order []gh.CheckRun
	row   int
	top   int

	declined string
}

// New builds the view for one pull request's checks.
func New(src Source, ref gh.ItemRef) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{src: src, ref: ref, loading: true, spin: s}
}

// Init starts the fetch.
func (m Model) Init() tea.Cmd { return tea.Batch(m.spin.Tick, m.fetch()) }

func (m Model) fetch() tea.Cmd {
	src, ref := m.src, m.ref
	return func() tea.Msg {
		c, err := src.PRChecks(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return errMsg{ref: ref, err: err}
		}
		return checksMsg{ref: ref, checks: c}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg), nil
	case checksMsg:
		return m.checksArrived(msg), nil
	case errMsg:
		return m.fetchFailed(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case spinner.TickMsg:
		return m.tick(msg)
	}
	return m, nil
}

func (m Model) resize(msg tea.WindowSizeMsg) Model {
	m.width, m.height = msg.Width, msg.Height
	return m
}

func (m Model) checksArrived(msg checksMsg) Model {
	// The request for the pull request the user just left is still in
	// flight; its answer must not replace this one's.
	if msg.ref != m.ref {
		return m
	}
	m.loading = false
	m.checks = msg.checks
	m.order = arrange(msg.checks.Runs)
	m.row = 0
	m.top = 0
	m.declined = ""
	return m
}

func (m Model) fetchFailed(msg errMsg) (Model, tea.Cmd) {
	if msg.ref != m.ref {
		return m, nil
	}
	m.loading = false
	return m, func() tea.Msg { return ErrorMsg{Err: msg.err} }
}

func (m Model) tick(msg spinner.TickMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spin, cmd = m.spin.Update(msg)
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		return m, func() tea.Msg { return ClosedMsg{} }
	case "r":
		m.loading = true
		m.declined = ""
		return m, tea.Batch(m.spin.Tick, m.fetch())
	case "j", "down":
		return m.moveRow(1), nil
	case "k", "up":
		return m.moveRow(-1), nil
	}
	return m, nil
}

func (m Model) moveRow(delta int) Model {
	m.row = clamp(m.row+delta, len(m.order)-1)
	m.declined = ""
	return m.follow()
}

// follow scrolls the window so the cursor stays on it.
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

// arrange puts the failing workflows at the top and keeps each workflow's
// checks together. A StatusContext belongs to no workflow and sorts last: it
// is the one the view can do the least with.
func arrange(runs []gh.CheckRun) []gh.CheckRun {
	worst := map[string]int{}
	first := map[string]int{}
	for i, r := range runs {
		worst[r.Workflow] = max(worst[r.Workflow], rank(r.State))
		if _, seen := first[r.Workflow]; !seen {
			first[r.Workflow] = i
		}
	}
	out := append([]gh.CheckRun(nil), runs...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Kind == gh.CheckKindStatus) != (b.Kind == gh.CheckKindStatus) {
			return b.Kind == gh.CheckKindStatus
		}
		if a.Workflow == b.Workflow {
			return false
		}
		if worst[a.Workflow] != worst[b.Workflow] {
			return worst[a.Workflow] > worst[b.Workflow]
		}
		// Two workflows in the same state still have to be told apart, or
		// their checks stay interleaved in whatever order GitHub listed them.
		return first[a.Workflow] < first[b.Workflow]
	})
	return out
}

func rank(s gh.CheckState) int {
	switch s {
	case gh.CheckFailure:
		return 3
	case gh.CheckRunning, gh.CheckPending:
		return 2
	default:
		return 1
	}
}

// clamp keeps v within [0, hi].
func clamp(v, hi int) int {
	if hi < 0 {
		return 0
	}
	return min(max(v, 0), hi)
}
