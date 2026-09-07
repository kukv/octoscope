package checks

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

type fakeSource struct {
	checks gh.Checks
	log    []gh.LogLine
}

func (f *fakeSource) PRChecks(context.Context, string, int) (gh.Checks, error) {
	return f.checks, nil
}

func (f *fakeSource) JobLog(context.Context, string, int64, bool) ([]gh.LogLine, error) {
	return f.log, nil
}

func (f *fakeSource) RerunWorkflow(context.Context, string, int64, gh.RerunScope) error { return nil }

func (f *fakeSource) OpenWeb(string) error { return nil }

// fixture is two workflows, the failing one recorded second on purpose: the
// view has to move it to the top.
func fixture() gh.Checks {
	return gh.Checks{
		Total: 3, Passed: 1, Failed: 1, Running: 1, State: gh.CheckFailure,
		Runs: []gh.CheckRun{
			{Name: "lint", State: gh.CheckSuccess, Kind: gh.CheckKindRun, Workflow: "CI", JobID: 1, RunID: 10},
			{Name: "sca", State: gh.CheckFailure, Kind: gh.CheckKindRun, Workflow: "security", JobID: 2, RunID: 20},
			{Name: "ci/circleci", State: gh.CheckRunning, Kind: gh.CheckKindStatus, URL: "https://circleci.example/1"},
		},
	}
}

func open(t *testing.T, width int) Model {
	t.Helper()

	m := New(&fakeSource{checks: fixture()}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	return m
}

// key builds the KeyPressMsg for a key name, matching the shape the app uses.
func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

func keyPress(s string) tea.KeyMsg { return key(s) }

func press(m Model, k string) Model {
	m, _ = m.Update(key(k))
	return m
}

func TestTheFailingWorkflowComesFirst(t *testing.T) {
	t.Parallel()

	view := open(t, 120).View()
	failing, passing := strings.Index(view, "sca"), strings.Index(view, "lint")
	if failing < 0 || passing < 0 {
		t.Fatalf("both checks should be drawn:\n%s", view)
	}
	if failing > passing {
		t.Errorf("the failing check is drawn below the passing one:\n%s", view)
	}
}

func TestTheHeaderCountsWhatIsWrong(t *testing.T) {
	t.Parallel()

	if view := open(t, 120).View(); !strings.Contains(view, "1 failing") {
		t.Errorf("header does not count the failures:\n%s", view)
	}
}

// interleaved is two green workflows whose checks GitHub listed alternately.
// Ranking alone cannot separate them, so this is what catches a comparator
// that only sorts by state.
func interleaved() gh.Checks {
	return gh.Checks{
		Total: 4, Passed: 4, State: gh.CheckSuccess,
		Runs: []gh.CheckRun{
			{Name: "a", State: gh.CheckSuccess, Kind: gh.CheckKindRun, Workflow: "CI", JobID: 1, RunID: 10},
			{Name: "b", State: gh.CheckSuccess, Kind: gh.CheckKindRun, Workflow: "release", JobID: 2, RunID: 20},
			{Name: "c", State: gh.CheckSuccess, Kind: gh.CheckKindRun, Workflow: "CI", JobID: 3, RunID: 10},
			{Name: "d", State: gh.CheckSuccess, Kind: gh.CheckKindRun, Workflow: "release", JobID: 4, RunID: 20},
		},
	}
}

func TestChecksOfOneWorkflowStayTogether(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{checks: interleaved()}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: interleaved()})
	var got []string
	for _, r := range m.order {
		got = append(got, r.Workflow)
	}
	want := []string{"CI", "CI", "release", "release"}
	if !slices.Equal(got, want) {
		t.Errorf("workflows in order = %v, want %v", got, want)
	}
}

func TestEscLeavesTheView(t *testing.T) {
	t.Parallel()

	_, cmd := open(t, 120).Update(keyPress("esc"))
	if cmd == nil {
		t.Fatal("esc produced no command, want ClosedMsg")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Errorf("esc sent %T, want ClosedMsg", cmd())
	}
}

// TestJMovesTheCursorDownTheList guards the list's own navigation: j/k are
// what the design table gives the row, and this task builds the list they
// move over.
func TestJMovesTheCursorDownTheList(t *testing.T) {
	t.Parallel()

	m := open(t, 120)
	before := m.row
	m = press(m, "j")
	if m.row == before {
		t.Errorf("j did not move the cursor off row %d", before)
	}
}

// manyChecks is four workflows of eight checks each: enough that the
// headings drawn between the groups push the cursor off a short screen.
func manyChecks() gh.Checks {
	var runs []gh.CheckRun
	id := int64(1)
	for _, wf := range []string{"alpha", "beta", "gamma", "delta"} {
		for i := range 8 {
			runs = append(runs, gh.CheckRun{
				Name:     fmt.Sprintf("%s-job%d", wf, i),
				State:    gh.CheckSuccess,
				Kind:     gh.CheckKindRun,
				Workflow: wf,
				JobID:    id,
				RunID:    id,
			})
			id++
		}
	}
	return gh.Checks{Total: len(runs), Passed: len(runs), State: gh.CheckSuccess, Runs: runs}
}

// TestTheCursorStaysOnScreenPastTheWorkflowHeadings guards the one thing the
// list has to keep true while it scrolls: the row the cursor is on is drawn.
// The headings take lines the cursor's own count knows nothing about, so a
// window scrolled by that count alone leaves the cursor behind.
func TestTheCursorStaysOnScreenPastTheWorkflowHeadings(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{checks: manyChecks()}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: manyChecks()})
	for range 20 {
		m = press(m, "j")
	}
	want := m.order[m.row].Name
	if view := m.View(); !strings.Contains(view, want) {
		t.Errorf("the selected check %q is not drawn anywhere:\n%s", want, view)
	}
}

// TestNoLineIsWiderThanTheTerminal guards the narrowest width the project
// promises to fit (.claude/rules/tui.md): every line of the view, including
// the key bar, must stay within the terminal's own column budget.
func TestNoLineIsWiderThanTheTerminal(t *testing.T) {
	t.Parallel()

	for _, line := range strings.Split(open(t, 80).View(), "\n") {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("line is %d columns wide, want at most 80:\n%s", w, line)
		}
	}
}

// moveTo presses j until the cursor sits on the check named name, and fails
// the test if it never does: a helper that returns silently when it cannot
// reach its target would make every test built on it pass for the wrong
// reason.
func moveTo(t *testing.T, m Model, name string) Model {
	t.Helper()

	for range m.order {
		if m.order[m.row].Name == name {
			return m
		}
		before := m.row
		m = press(m, "j")
		if m.row == before {
			t.Fatalf("moveTo(%q): j stopped moving at row %d before reaching it", name, m.row)
		}
	}
	t.Fatalf("moveTo(%q): no such check in the fixture", name)
	return m
}

func TestALogForACheckTheUserLeftIsDropped(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: fixture(), log: []gh.LogLine{{Step: "s", Text: "FAIL sca"}}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	_, cmd := m.Update(keyPress("enter")) // asks for the log of the selected check
	m = press(m, "j")                     // and the user moves on before it lands
	m, _ = m.Update(cmd())
	if view := m.View(); strings.Contains(view, "FAIL sca") {
		t.Errorf("the log of the check the user left is drawn under another one:\n%s", view)
	}
}

// TestMovingTheCursorClearsTheLogUnderIt guards the pairing of the two
// panes: a log that has already landed carries nothing on screen saying
// whose it is, so leaving it under another check misreads as that check's.
func TestMovingTheCursorClearsTheLogUnderIt(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: fixture(), log: []gh.LogLine{{Step: "s", Text: "FAIL sca"}}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	m = press(m, "j")
	if view := m.View(); strings.Contains(view, "FAIL sca") {
		t.Errorf("the log stayed on screen under the next check:\n%s", view)
	}
}

func TestEnterFetchesTheLogOfTheSelectedCheck(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: fixture(), log: []gh.LogLine{{Step: "Run tests", Text: "FAIL ./internal/gh"}}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("enter started no fetch")
	}
	m, _ = m.Update(cmd())
	if view := m.View(); !strings.Contains(view, "FAIL ./internal/gh") {
		t.Errorf("the log is not on screen:\n%s", view)
	}
}

func TestASucceededJobSaysNoStepFailed(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: fixture()} // JobLog answers with no lines
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	if view := m.View(); !strings.Contains(view, i18n.T("checks.log_empty_failed")) {
		t.Errorf("an empty failed-step log said nothing:\n%s", view)
	}
}

func TestAStatusContextSaysWhyItHasNoLog(t *testing.T) {
	t.Parallel()

	m := open(t, 120)
	m = moveTo(t, m, "ci/circleci")
	m, _ = m.Update(keyPress("enter"))
	if view := m.View(); !strings.Contains(view, i18n.T("checks.decline_status_context")) {
		t.Errorf("enter on a StatusContext did nothing and said nothing:\n%s", view)
	}
}

// TestTheLogPaneStopsScrollingAtItsWidestLine guards the right-hand edge: an
// unbounded l walks the window past every line there is and leaves the pane
// blank, with nothing on screen saying how to get back.
func TestTheLogPaneStopsScrollingAtItsWidestLine(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 400)
	src := &fakeSource{checks: fixture(), log: []gh.LogLine{{Step: "s", Text: long}}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	for range 500 {
		m = press(m, "l")
	}
	if view := m.View(); !strings.Contains(view, "xxx") {
		t.Errorf("l scrolled the log pane off the end of its own lines:\n%s", view)
	}
}

func TestEnterOnAPullRequestWithNoChecksSaysSo(t *testing.T) {
	t.Parallel()

	m := openChecks(t, gh.Checks{}, 120, 30)
	m, _ = m.Update(keyPress("enter"))
	if m.declined != i18n.T("checks.none") {
		t.Errorf("declined = %q, want %q: the fetch has landed, nothing is loading",
			m.declined, i18n.T("checks.none"))
	}
}

func TestRerunOnAPullRequestWithNoChecksSaysSo(t *testing.T) {
	t.Parallel()

	m := press(openChecks(t, gh.Checks{}, 120, 30), "R")
	if m.declined != i18n.T("checks.none") {
		t.Errorf("declined = %q, want %q: the fetch has landed, nothing is loading",
			m.declined, i18n.T("checks.none"))
	}
}

func TestOpenOnACheckWithNoPageOfItsOwnSaysWhy(t *testing.T) {
	t.Parallel()

	m := moveTo(t, open(t, 120), "lint") // the fixture records it with no URL
	m, _ = m.Update(keyPress("o"))
	if view := m.View(); !strings.Contains(view, i18n.T("checks.decline_no_url")) {
		t.Errorf("o on a check with no page did nothing and said nothing:\n%s", view)
	}
}

// appCheck is one check run an App created: GitHub reports those with a null
// checkSuite.workflowRun, so there is no workflow behind them to name or to
// rerun (Codecov, Sonar and deploy checks are all this shape).
func appCheck() gh.Checks {
	return gh.Checks{
		Total: 1, Passed: 1, State: gh.CheckSuccess,
		Runs: []gh.CheckRun{
			{Name: "codecov/patch", State: gh.CheckSuccess, Kind: gh.CheckKindRun, JobID: 7},
		},
	}
}

func openChecks(t *testing.T, c gh.Checks, width, height int) Model {
	t.Helper()

	m := New(&fakeSource{checks: c}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: c})
	return m
}

func TestACheckWithNoWorkflowRunGetsNoHeading(t *testing.T) {
	t.Parallel()

	m := openChecks(t, appCheck(), 120, 30)
	if rows := m.allRows(); len(rows) != 1 {
		t.Errorf("the list drew %d lines, want 1: a check with no workflow run has no heading:\n%s",
			len(rows), strings.Join(rows, "\n"))
	}
}

func TestTheLogDoesNotWrap(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 400)
	src := &fakeSource{checks: fixture(), log: []gh.LogLine{{Step: "s", Text: long}}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 80 {
			t.Fatalf("line %d is %d columns wide, want at most 80:\n%s", i, w, line)
		}
	}
}
