package work

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

type fakeSource struct {
	work gh.Work
	err  error

	// sections is every column that was asked for, in the order the requests
	// were made. Each column is its own request now, so "which columns were
	// asked for" is a thing a test has to be able to say.
	sections []gh.WorkSection
}

func (f *fakeSource) ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error) {
	f.sections = append(f.sections, s)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return f.work[s], f.err
}

func sampleWork() gh.Work {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	var w gh.Work
	w[gh.SectionReviewRequested] = []gh.WorkItem{
		{
			Ref:   gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 12},
			Title: "fix the thing", UpdatedAt: now,
			// CRLF on purpose: GitHub returns whatever line endings the author
			// used, and a carriage return left in a drawn line shifts it.
			Body:   "The renderer dropped every escape.\r\n\r\nThis puts them back.",
			Labels: []gh.Label{{Name: "bug", Color: "d73a4a"}, {Name: "ci", Color: "d4c5f9"}},
			Head:   "feat/graph", Base: "main", Additions: 218, Deletions: 31,
			Checks: gh.Checks{
				Total: 3, Passed: 1, Failed: 1, Running: 1, State: gh.CheckFailure,
				Runs: []gh.CheckRun{
					{Name: "build", State: gh.CheckSuccess},
					{Name: "lint", State: gh.CheckRunning},
					{Name: "test", State: gh.CheckFailure},
				},
			},
		},
		{
			Ref:   gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 3},
			Title: "bump deps", UpdatedAt: now,
		},
	}
	w[gh.SectionAssigned] = []gh.WorkItem{
		{
			Ref:   gh.ItemRef{Kind: gh.ItemIssue, Repo: "kukv/octoscope", Number: 7},
			Title: "an issue", UpdatedAt: now,
		},
	}
	return w
}

// sampleItems is the column sampleWork fills with cards.
func sampleItems() []gh.WorkItem { return sampleWork()[gh.SectionReviewRequested] }

// loaded returns a model that already received its data.
func loaded() Model {
	return answered(sized(New(&fakeSource{work: sampleWork()})), sampleWork())
}

// sized gives a board the size it needs before it can draw: wide enough for
// all four columns and the drawer.
func sized(m Model) Model {
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

// answered hands the board every column of w, one answer at a time, the way
// four separate requests arrive.
func answered(m Model, w gh.Work) Model {
	for _, s := range gh.WorkSections() {
		m, _ = m.Update(workMsg{section: s, items: w[s]})
	}
	return m
}

// key builds the KeyPressMsg for a key name, matching the shape the app uses.
func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

// drain runs cmd and every command in the batch it produced, flattening the
// result into the messages they returned. Refresh batches one command per
// column now, so picking the fetch out by position would be reading the
// implementation rather than testing it.
func drain(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		msgs = append(msgs, drain(t, c)...)
	}
	return msgs
}

// fetchMsgs is what the fetches reported, with the spinner's own tick left
// out: it says nothing about the fetch and reports forever.
func fetchMsgs(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	var msgs []tea.Msg
	for _, msg := range drain(t, cmd) {
		if _, tick := msg.(spinner.TickMsg); !tick {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

func press(m Model, k string) Model {
	m, _ = m.Update(key(k))
	return m
}

func TestCursorMovesWithinAColumn(t *testing.T) {
	m := press(loaded(), "j")
	if m.row != 1 {
		t.Errorf("row: got %d, want 1", m.row)
	}
	if m = press(m, "j"); m.row != 1 {
		t.Errorf("row past the end: got %d, want it clamped to 1", m.row)
	}
	if m = press(press(m, "k"), "k"); m.row != 0 {
		t.Errorf("row before the start: got %d, want it clamped to 0", m.row)
	}
}

func TestMovingToAShorterColumnClampsTheRow(t *testing.T) {
	m := press(loaded(), "j") // row 1 of the 2-item first column
	m = press(m, "l")         // column 1 is empty
	m = press(m, "l")         // column 2 holds one item
	if m.col != 2 {
		t.Fatalf("col: got %d, want 2", m.col)
	}
	if m.row != 0 {
		t.Errorf("row: got %d, want it clamped to 0", m.row)
	}
}

func TestColumnWrapsAtBothEnds(t *testing.T) {
	if m := press(loaded(), "h"); m.col != 3 {
		t.Errorf("h from column 0: got %d, want 3", m.col)
	}
	m := loaded()
	for range 4 {
		m = press(m, "l")
	}
	if m.col != 0 {
		t.Errorf("four l presses: got %d, want 0", m.col)
	}
}

func TestSelectedRefNamesTheItemUnderTheCursor(t *testing.T) {
	ref, ok := press(loaded(), "j").SelectedRef()
	if !ok {
		t.Fatal("SelectedRef reported no selection")
	}
	if ref.Repo != "kukv/koto" || ref.Number != 3 {
		t.Errorf("got %s#%d, want kukv/koto#3", ref.Repo, ref.Number)
	}
}

func TestEmptyColumnHasNoSelection(t *testing.T) {
	if _, ok := press(loaded(), "l").SelectedRef(); ok {
		t.Error("SelectedRef reported a selection in an empty column")
	}
}

func TestEnterAsksTheParentToOpenTheDetail(t *testing.T) {
	_, cmd := loaded().Update(key("enter"))
	if cmd == nil {
		t.Fatal("enter produced no command")
	}
	msg, ok := cmd().(OpenDetailMsg)
	if !ok {
		t.Fatalf("got %T, want OpenDetailMsg", cmd())
	}
	if msg.Ref.Number != 12 {
		t.Errorf("got #%d, want #12", msg.Ref.Number)
	}
}

func TestEnterOnAnEmptyColumnDoesNothing(t *testing.T) {
	if _, cmd := press(loaded(), "l").Update(key("enter")); cmd != nil {
		t.Error("enter produced a command with nothing selected")
	}
}

func TestDAsksForTheDiff(t *testing.T) {
	m := loaded()
	_, cmd := m.Update(key("d"))
	if cmd == nil {
		t.Fatal("d produced no command")
	}
	msg, ok := cmd().(OpenDiffMsg)
	if !ok {
		t.Fatalf("got %T, want OpenDiffMsg", cmd())
	}
	want, _ := m.SelectedRef()
	if msg.Ref != want {
		t.Errorf("d asked for %+v, want the selected card %+v", msg.Ref, want)
	}
}

// TestDDoesNothingOnAnIssue is what stops the diff view opening on something
// that has no diff.
func TestDDoesNothingOnAnIssue(t *testing.T) {
	m := loaded()
	m = press(m, "l") // column 1 (your PRs) is empty
	m = press(m, "l") // column 2 (assigned) holds the issue
	if ref, ok := m.SelectedRef(); !ok || ref.Kind != gh.ItemIssue {
		t.Fatalf("selection = %+v, ok=%v, want the issue", ref, ok)
	}
	if _, cmd := m.Update(key("d")); cmd != nil {
		t.Errorf("d on an issue produced %T", cmd())
	}
}

func TestSAsksForTheChecks(t *testing.T) {
	m := loaded()
	_, cmd := m.Update(key("s"))
	if cmd == nil {
		t.Fatal("s produced no command")
	}
	msg, ok := cmd().(OpenChecksMsg)
	if !ok {
		t.Fatalf("got %T, want OpenChecksMsg", cmd())
	}
	want, _ := m.SelectedRef()
	if msg.Ref != want {
		t.Errorf("s asked for %+v, want the selected card %+v", msg.Ref, want)
	}
}

// TestSDoesNothingOnAnIssue is what stops the checks view opening on
// something that has no checks.
func TestSDoesNothingOnAnIssue(t *testing.T) {
	m := loaded()
	m = press(m, "l") // column 1 (your PRs) is empty
	m = press(m, "l") // column 2 (assigned) holds the issue
	if ref, ok := m.SelectedRef(); !ok || ref.Kind != gh.ItemIssue {
		t.Fatalf("selection = %+v, ok=%v, want the issue", ref, ok)
	}
	if _, cmd := m.Update(key("s")); cmd != nil {
		t.Errorf("s on an issue produced %T", cmd())
	}
}

func TestFetchFailureBecomesAnErrorMsg(t *testing.T) {
	m := New(&fakeSource{err: errors.New("boom")})
	_, cmd := m.Update(errMsg{section: gh.SectionAssigned, err: errors.New("boom")})
	if cmd == nil {
		t.Fatal("no command returned for a failed fetch")
	}
	got, ok := cmd().(ErrorMsg)
	if !ok {
		t.Fatalf("got %T, want ErrorMsg", cmd())
	}
	if got.Err.Error() != "boom" {
		t.Errorf("got %q, want boom", got.Err)
	}
}

func TestAFetchThatFailsOnItsOwnStillReports(t *testing.T) {
	m := New(&fakeSource{err: errors.New("boom")})
	m, cmd := m.Refresh()
	t.Cleanup(m.Cancel)
	if cmd == nil {
		t.Fatal("Refresh returned no command")
	}
	msgs := fetchMsgs(t, cmd)
	if len(msgs) != gh.WorkSectionCount {
		t.Fatalf("%d columns reported, want %d", len(msgs), gh.WorkSectionCount)
	}
	for _, msg := range msgs {
		got, ok := msg.(errMsg)
		if !ok {
			t.Fatalf("got %T, want errMsg", msg)
		}
		if got.err.Error() != "boom" {
			t.Errorf("got %q, want boom", got.err)
		}
	}
}

// Each column is its own request, so each has to be asked for.
func TestRefreshAsksForEveryColumn(t *testing.T) {
	f := &fakeSource{work: sampleWork()}
	m, cmd := New(f).Refresh()
	t.Cleanup(m.Cancel)
	fetchMsgs(t, cmd)

	if len(f.sections) != gh.WorkSectionCount {
		t.Fatalf("asked for %d columns, want %d", len(f.sections), gh.WorkSectionCount)
	}
	seen := map[gh.WorkSection]bool{}
	for _, s := range f.sections {
		seen[s] = true
	}
	for _, s := range gh.WorkSections() {
		if !seen[s] {
			t.Errorf("column %d was never asked for", s)
		}
	}
}

func TestRefreshCancelsThePreviousFetch(t *testing.T) {
	f := &fakeSource{work: sampleWork()}
	m := New(f)

	m, first := m.Refresh()
	m, second := m.Refresh()
	t.Cleanup(m.Cancel)

	if first == nil || second == nil {
		t.Fatal("Refresh returned no command")
	}
	// The second Refresh cancelled the first one's context. A cancelled fetch
	// reports nothing at all: an error screen for a refresh the user asked for
	// would be worse than silence.
	for _, msg := range fetchMsgs(t, first) {
		if msg != nil {
			t.Errorf("first fetch: got %T, want no message", msg)
		}
	}
	for _, msg := range fetchMsgs(t, second) {
		if _, ok := msg.(workMsg); !ok {
			t.Errorf("second fetch: got %T, want workMsg", msg)
		}
	}
	if want := 2 * gh.WorkSectionCount; len(f.sections) != want {
		t.Errorf("ListWorkSection calls: got %d, want %d", len(f.sections), want)
	}
}

func TestRefreshMarksEveryColumnLoading(t *testing.T) {
	m := New(&fakeSource{work: sampleWork()})
	for _, s := range gh.WorkSections() {
		if m.loading[s] {
			t.Errorf("New returned column %d loading; the parent starts the first fetch", s)
		}
	}
	m, _ = m.Refresh()
	t.Cleanup(m.Cancel)
	for _, s := range gh.WorkSections() {
		if !m.loading[s] {
			t.Errorf("Refresh did not mark column %d loading", s)
		}
	}

	// The columns share one context, so it is only safe to let go of once the
	// last of them has answered.
	m, _ = m.Update(workMsg{section: gh.SectionReviewRequested, items: sampleItems()})
	if m.cancel == nil {
		t.Error("the context was released while three columns were still running")
	}
	m = answered(m, sampleWork())
	for _, s := range gh.WorkSections() {
		if m.loading[s] {
			t.Errorf("column %d is still loading after its data arrived", s)
		}
	}
	if m.cancel != nil {
		t.Error("the finished fetch's context is still held")
	}
}

func TestRKeyRefetchesTheBoard(t *testing.T) {
	f := &fakeSource{work: sampleWork()}
	m := answered(New(f), sampleWork())

	m, cmd := m.Update(key("r"))
	t.Cleanup(m.Cancel)
	if cmd == nil {
		t.Fatal("r produced no command")
	}
	for _, s := range gh.WorkSections() {
		if !m.loading[s] {
			t.Errorf("r did not mark column %d loading", s)
		}
	}
	for _, msg := range fetchMsgs(t, cmd) {
		if _, ok := msg.(workMsg); !ok {
			t.Errorf("got %T, want workMsg", msg)
		}
	}
	if len(f.sections) != gh.WorkSectionCount {
		t.Errorf("ListWorkSection calls: got %d, want %d", len(f.sections), gh.WorkSectionCount)
	}
}

// The columns answer at very different speeds, so what has arrived is drawn
// rather than held back for the slowest.
func TestAColumnIsDrawnBeforeTheOthersArrive(t *testing.T) {
	m := sized(New(&fakeSource{}))
	m, _ = m.Refresh()
	t.Cleanup(m.Cancel)

	m, _ = m.Update(workMsg{section: gh.SectionAssigned, items: sampleItems()})
	view := ansi.Strip(m.View())
	if !strings.Contains(view, sampleItems()[0].Title) {
		t.Errorf("the column that answered is not on screen:\n%s", view)
	}
	if !strings.Contains(view, i18n.T("common.loading")) {
		t.Errorf("the columns still waiting say nothing:\n%s", view)
	}
	// The waiting columns keep their frame: the spinner goes where their
	// cards will go, not over the board.
	if !strings.Contains(view, i18n.T("work.mentioned")) {
		t.Errorf("a waiting column lost its heading:\n%s", view)
	}
}

// One column failing must not cost the other three.
func TestAFailedColumnLeavesTheOthersAlone(t *testing.T) {
	m := sized(New(&fakeSource{}))
	m, _ = m.Update(workMsg{section: gh.SectionAssigned, items: sampleItems()})
	m, _ = m.Update(errMsg{section: gh.SectionYourPRs, err: errors.New("gh: HTTP 502")})
	if view := ansi.Strip(m.View()); !strings.Contains(view, sampleItems()[0].Title) {
		t.Errorf("a failure in one column emptied another:\n%s", view)
	}
}

// The tab row reports the board's age. Until every column has answered the
// number would describe part of a board, so it is not shown at all.
func TestTheSummaryWaitsForEveryColumn(t *testing.T) {
	m := sized(New(&fakeSource{}))
	for _, s := range gh.WorkSections()[:gh.WorkSectionCount-1] {
		m, _ = m.Update(workMsg{section: s, items: nil})
	}
	if m.Summary().Ready {
		t.Error("the summary was ready before the last column answered")
	}
	m, _ = m.Update(workMsg{section: gh.WorkSections()[gh.WorkSectionCount-1], items: nil})
	if !m.Summary().Ready {
		t.Error("the summary never became ready")
	}
}

// The age on screen is the age of the oldest thing on screen.
func TestTheSummaryReportsTheOldestColumn(t *testing.T) {
	m := sized(New(&fakeSource{}))
	m = answered(m, gh.Work{})

	// The oldest column sits in the middle on purpose: returning the first
	// column's time, the last one's, or the newest are all shapes a wrong
	// Summary takes, and each has to be told apart from the right answer.
	base := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	want := base.Add(-time.Hour)
	m.fetchedAt = [gh.WorkSectionCount]time.Time{
		base, base.Add(time.Minute), want, base.Add(time.Hour),
	}
	if got := m.Summary().FetchedAt; !got.Equal(want) {
		t.Errorf("the summary reports %v, want the oldest column's %v", got, want)
	}
}

func TestNewDataClampsTheCursor(t *testing.T) {
	m := press(loaded(), "j")
	m, _ = m.Update(workMsg{section: gh.SectionReviewRequested})
	if m.row != 0 {
		t.Errorf("row after the data was replaced: got %d, want 0", m.row)
	}
	if _, ok := m.SelectedRef(); ok {
		t.Error("SelectedRef reported a selection in an emptied column")
	}
}
