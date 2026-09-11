// Package work implements the Work board: the columns of what needs the
// user's attention, across every repository they touch.
package work

import (
	"context"
	"slices"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// Source is what the Work board needs from the GitHub layer.
type Source interface {
	ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error)
}

type (
	workMsg struct {
		section gh.WorkSection
		items   []gh.WorkItem
	}
	errMsg struct {
		section gh.WorkSection
		err     error
	}
)

// OpenDetailMsg asks the parent to show the detail view for the selected card.
type OpenDetailMsg struct{ Ref gh.ItemRef }

// OpenDiffMsg asks the parent to show the diff of the selected pull request.
type OpenDiffMsg struct{ Ref gh.ItemRef }

// OpenChecksMsg asks the parent to show the checks of the selected pull
// request.
type OpenChecksMsg struct{ Ref gh.ItemRef }

// FatalMsg carries a failure the parent shows on its error screen. Only what
// the user has to act on travels this way; everything else stays on the board
// as a notice (see gh.IsFatal).
type FatalMsg struct{ Err error }

// colState is where one column stands. It is one value rather than a bool
// per condition: "loading" and "answered" as two bools name four states of
// which one is nonsense, and every reader would have to know which
// (.claude/rules/tui.md).
type colState uint8

const (
	colUnfetched colState = iota // nothing has been asked for yet
	colLoading
	colLoaded
	colFailed
)

// answered reports whether the column has come back at all. A failure counts:
// it is not coming back on its own, and the board cannot go on waiting for it.
func (s colState) answered() bool { return s == colLoaded || s == colFailed }

type Model struct {
	src Source

	width, height int
	spin          spinner.Model
	work          gh.Work
	col, row      int

	// state is where each column stands. Each column is its own request and
	// they answer at very different speeds, so the board is almost never in
	// one state as a whole.
	state [gh.WorkSectionCount]colState

	// notice is what GitHub said about a column the board carries on without:
	// the previous answer is still on screen and r asks again. A successful
	// fetch clears it, so a stale complaint never outlives what it described.
	notice [gh.WorkSectionCount]string

	// fetchedAt is when each column's data arrived. The cards show relative
	// times, and View must render the same string from the same state, so the
	// clock is read once in Update rather than on every draw.
	fetchedAt [gh.WorkSectionCount]time.Time

	// cancel stops the in-flight fetch. The board is the one place where a
	// request outlives the user's interest in it: they can switch tabs or ask
	// for a refresh while four searches are still running.
	cancel context.CancelFunc
}

func New(src Source) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{src: src, spin: s}
}

// Refresh cancels any in-flight fetch and starts a new one. It is also how
// the first fetch is started: a tea.Cmd-returning Init could not hand the
// cancel function back to the caller.
func (m Model) Refresh() (Model, tea.Cmd) {
	m.Cancel()
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	// The spinner ticks from here rather than from an Init the board does not
	// have: the animation starts with the fetch it belongs to.
	cmds := []tea.Cmd{m.spin.Tick}
	// The notices go with the answers they described: a column that is being
	// asked again has no failure to report until the new request answers.
	m.notice = [gh.WorkSectionCount]string{}
	for _, s := range gh.WorkSections() {
		m.state[s] = colLoading
		cmds = append(cmds, fetchSection(ctx, m.src, s))
	}
	return m, tea.Batch(cmds...)
}

// fetchSection asks for one column. The four columns share a context so one
// Cancel still stops all of them, but they travel as four requests: asking
// for all four in one is what made GitHub's front end stop answering.
func fetchSection(ctx context.Context, src Source, s gh.WorkSection) tea.Cmd {
	return func() tea.Msg {
		items, err := src.ListWorkSection(ctx, s)
		// A cancelled fetch is not a failure: the user refreshed, left the tab
		// or quit. Bubble Tea drops a nil message, so the stale fetch reports
		// nothing instead of an error screen. The context is what says so —
		// cancelling a gh subprocess surfaces as "signal: killed", not as a
		// wrapped context.Canceled.
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return errMsg{section: s, err: err}
		}
		return workMsg{section: s, items: items}
	}
}

// Cancel stops the in-flight fetch. The parent calls it when the user quits.
func (m Model) Cancel() {
	if m.cancel != nil {
		m.cancel()
	}
}

// releaseFetch frees the context once every column has answered, so a
// finished board leaves nothing behind to cancel. The four columns share the
// context, so dropping it on the first answer would orphan the other three.
func (m *Model) releaseFetch() {
	if slices.Contains(m.state[:], colLoading) {
		return
	}
	m.Cancel()
	m.cancel = nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case workMsg:
		m.state[msg.section] = colLoaded
		m.notice[msg.section] = ""
		m.releaseFetch()
		m.work[msg.section] = msg.items
		m.fetchedAt[msg.section] = time.Now()
		m.clampCursor()
	case errMsg:
		m.state[msg.section] = colFailed
		m.releaseFetch()
		if gh.IsFatal(msg.err) {
			err := msg.err
			return m, func() tea.Msg { return FatalMsg{err} }
		}
		m.notice[msg.section] = msg.err.Error()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "h", "left":
		m.col = wrapColumn(m.col-1, m.columns())
		m.clampCursor()
	case "l", "right":
		m.col = wrapColumn(m.col+1, m.columns())
		m.clampCursor()
	case "j", "down":
		if m.row+1 < len(m.work[m.section()]) {
			m.row++
		}
	case "k", "up":
		if m.row > 0 {
			m.row--
		}
	case "r":
		return m.Refresh()
	case "enter":
		if ref, ok := m.SelectedRef(); ok {
			return m, func() tea.Msg { return OpenDetailMsg{ref} }
		}
	case "d":
		ref, ok := m.SelectedRef()
		// An issue has no diff. Opening an empty diff view would be a worse
		// answer than doing nothing.
		if !ok || ref.Kind != gh.ItemPR {
			return m, nil
		}
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
	case "s":
		ref, ok := m.SelectedRef()
		// An issue has no checks.
		if !ok || ref.Kind != gh.ItemPR {
			return m, nil
		}
		return m, func() tea.Msg { return OpenChecksMsg{Ref: ref} }
	}
	return m, nil
}

func (m Model) columns() int { return len(gh.WorkSections()) }

func (m Model) section() gh.WorkSection { return gh.WorkSections()[m.col] }

func wrapColumn(i, n int) int {
	switch {
	case i < 0:
		return n - 1
	case i >= n:
		return 0
	default:
		return i
	}
}

// clampCursor pulls the row back into range after the column changed or the
// data was replaced. It takes a pointer because it is only ever called on the
// local copy handleKey and Update are about to return.
func (m *Model) clampCursor() {
	n := len(m.work[m.section()])
	if m.row >= n {
		m.row = max(n-1, 0)
	}
}

// Summary is what the tab row reports about the board: how much is waiting on
// the user, how much is broken, and when the board last heard from GitHub.
// The root draws it, so the board hands it over rather than drawing it in a
// place that is not its own.
type Summary struct {
	Attention int
	Failing   int
	FetchedAt time.Time
	Ready     bool
}

// Summary counts the board. Attention is what has been asked of the user;
// Failing is every pull request whose checks are red, wherever it sits.
func (m Model) Summary() Summary {
	s := Summary{FetchedAt: m.oldestFetch(), Ready: m.ready()}
	s.Attention = len(m.work[gh.SectionReviewRequested])
	for _, items := range m.work {
		for _, it := range items {
			if it.Checks.State == gh.CheckFailure {
				s.Failing++
			}
		}
	}
	return s
}

// ready reports whether the counts are worth showing. Until every column has
// answered they would describe part of a board -- and a refresh puts every
// column back to waiting, so the old counts do not outlive the board they
// counted.
func (m Model) ready() bool {
	for _, s := range m.state {
		if !s.answered() {
			return false
		}
	}
	return true
}

// oldestFetch is the age on screen: the age of the oldest thing on it. The
// columns answer seconds apart, and naming the newest would claim the board
// is fresher than what the user is looking at.
func (m Model) oldestFetch() time.Time {
	var oldest time.Time
	for _, at := range m.fetchedAt {
		if at.IsZero() {
			continue
		}
		if oldest.IsZero() || at.Before(oldest) {
			oldest = at
		}
	}
	return oldest
}

// SelectedRef names the card under the cursor. ok is false when the column is
// empty.
func (m Model) SelectedRef() (gh.ItemRef, bool) {
	items := m.work[m.section()]
	if m.row >= len(items) {
		return gh.ItemRef{}, false
	}
	return items[m.row].Ref, true
}
