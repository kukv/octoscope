package checks

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/i18n"
)

type fakeSource struct {
	checks domain.Checks
	log    []domain.LogLine
}

func (f *fakeSource) PRChecks(context.Context, string, int) (domain.Checks, error) {
	return f.checks, nil
}

func (f *fakeSource) JobLog(context.Context, string, domain.JobHandle, bool) ([]domain.LogLine, error) {
	return f.log, nil
}

func (f *fakeSource) RerunWorkflow(context.Context, string, domain.RunHandle, domain.RerunScope) error {
	return nil
}

// fixture is two workflows, the failing one recorded second on purpose: the
// view has to move it to the top.
func fixture() domain.Checks {
	return domain.Checks{
		Total: 3, Passed: 1, Failed: 1, Running: 1, State: domain.CheckFailure,
		Runs: []domain.CheckRun{
			{Name: "lint", State: domain.CheckSuccess, Kind: domain.CheckKindRun, Workflow: "CI", Job: "1", WorkflowRun: "10"},
			{Name: "sca", State: domain.CheckFailure, Kind: domain.CheckKindRun, Workflow: "security", Job: "2", WorkflowRun: "20"},
			{Name: "ci/circleci", State: domain.CheckRunning, Kind: domain.CheckKindStatus, URL: "https://circleci.example/1"},
		},
	}
}

func open(t *testing.T, width int) Model {
	t.Helper()

	m := New(&fakeSource{checks: fixture()}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	return m
}

// keyPress builds the KeyPressMsg for a key name, matching the shape the app
// uses.
func keyPress(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

func press(m Model, k string) Model {
	m, _ = m.Update(keyPress(k))
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
func interleaved() domain.Checks {
	return domain.Checks{
		Total: 4, Passed: 4, State: domain.CheckSuccess,
		Runs: []domain.CheckRun{
			{Name: "a", State: domain.CheckSuccess, Kind: domain.CheckKindRun, Workflow: "CI", Job: "1", WorkflowRun: "10"},
			{Name: "b", State: domain.CheckSuccess, Kind: domain.CheckKindRun, Workflow: "release", Job: "2", WorkflowRun: "20"},
			{Name: "c", State: domain.CheckSuccess, Kind: domain.CheckKindRun, Workflow: "CI", Job: "3", WorkflowRun: "10"},
			{Name: "d", State: domain.CheckSuccess, Kind: domain.CheckKindRun, Workflow: "release", Job: "4", WorkflowRun: "20"},
		},
	}
}

func TestChecksOfOneWorkflowStayTogether(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{checks: interleaved()}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
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
// what the design table gives the row.
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
func manyChecks() domain.Checks {
	var runs []domain.CheckRun
	id := 1
	for _, wf := range []string{"alpha", "beta", "gamma", "delta"} {
		for i := range 8 {
			runs = append(runs, domain.CheckRun{
				Name:        fmt.Sprintf("%s-job%d", wf, i),
				State:       domain.CheckSuccess,
				Kind:        domain.CheckKindRun,
				Workflow:    wf,
				Job:         domain.JobHandle(strconv.Itoa(id)),
				WorkflowRun: domain.RunHandle(strconv.Itoa(id)),
			})
			id++
		}
	}
	return domain.Checks{Total: len(runs), Passed: len(runs), State: domain.CheckSuccess, Runs: runs}
}

// TestTheCursorStaysOnScreenPastTheWorkflowHeadings guards the one thing the
// list has to keep true while it scrolls: the row the cursor is on is drawn.
// The headings take lines the cursor's own count knows nothing about, so a
// window scrolled by that count alone leaves the cursor behind.
func TestTheCursorStaysOnScreenPastTheWorkflowHeadings(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{checks: manyChecks()}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
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

// TestTheHeadingRuleFitsATerminalNarrowerThanItself guards the one line the
// view builds out of fixed parts -- the list's own heading, the divider and
// the log pane's mode label -- rather than out of what is left of the width.
// Below about 35 columns those parts alone are wider than the terminal.
func TestTheHeadingRuleFitsATerminalNarrowerThanItself(t *testing.T) {
	t.Parallel()

	for _, line := range strings.Split(open(t, 30).View(), "\n") {
		if w := ansi.StringWidth(line); w > 30 {
			t.Errorf("line is %d columns wide, want at most 30:\n%s", w, line)
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

	src := &fakeSource{checks: fixture(), log: []domain.LogLine{{Step: "s", Text: "FAIL sca"}}}
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
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

	src := &fakeSource{checks: fixture(), log: []domain.LogLine{{Step: "s", Text: "FAIL sca"}}}
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
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

	src := &fakeSource{checks: fixture(), log: []domain.LogLine{{Step: "Run tests", Text: "FAIL ./internal/gh"}}}
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
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
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	if view := m.View(); !strings.Contains(view, i18n.T("checks.log_empty_failed")) {
		t.Errorf("an empty failed-step log said nothing:\n%s", view)
	}
}

// TestAnEmptyFullLogSaysSo is the other half of the empty-log pair: the
// failed-steps view says no step failed, and a full log that came back empty
// left the pane blank with nothing saying whether it had even been asked for.
func TestAnEmptyFullLogSaysSo(t *testing.T) {
	t.Parallel()

	src := &fakeSource{checks: fixture()} // JobLog answers with no lines
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("L"))
	m, _ = m.Update(cmd())
	if view := m.View(); !strings.Contains(view, i18n.T("checks.log_empty_full")) {
		t.Errorf("an empty full log said nothing:\n%s", view)
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
	src := &fakeSource{checks: fixture(), log: []domain.LogLine{{Step: "s", Text: long}}}
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
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

// TestMovingDownOntoShorterLinesBringsTheLogBack guards the other end of the
// horizontal bound: an offset that was inside the widest line on screen is
// past every line once the window moves onto narrower ones, and a blank pane
// says nothing about how to get back.
func TestMovingDownOntoShorterLinesBringsTheLogBack(t *testing.T) {
	t.Parallel()

	log := []domain.LogLine{
		{Step: "s", Text: strings.Repeat("x", 400)},
		{Step: "s", Text: "alpha"},
		{Step: "s", Text: "bravo"},
		{Step: "s", Text: "charlie"},
		{Step: "s", Text: "delta"},
	}
	src := &fakeSource{checks: fixture(), log: log}
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	for range 500 {
		m = press(m, "l") // the first press moves the cursor to the log pane
	}
	for range 3 {
		m = press(m, "j")
	}
	if view := m.View(); !strings.Contains(view, "charlie") {
		t.Errorf("the log pane is blank after moving down onto shorter lines:\n%s", view)
	}
}

// TestAWiderTerminalBringsTheLogBack is the other way the window moves under
// a standing offset: the pane grows until it could hold the whole line, and
// an offset the narrow pane needed would still be cutting the start off.
func TestAWiderTerminalBringsTheLogBack(t *testing.T) {
	t.Parallel()

	src := &fakeSource{
		checks: fixture(),
		log:    []domain.LogLine{{Step: "s", Text: "START" + strings.Repeat("x", 400)}},
	}
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 8})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m, cmd := m.Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	for range 500 {
		m = press(m, "l")
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 480, Height: 8})
	if view := m.View(); !strings.Contains(view, "START") {
		t.Errorf("the log pane is wide enough for the whole line but still scrolled past its start:\n%s", view)
	}
}

func TestEnterOnAPullRequestWithNoChecksSaysSo(t *testing.T) {
	t.Parallel()

	m := openChecks(t, 120, 30, domain.Checks{})
	m, _ = m.Update(keyPress("enter"))
	if m.declined != i18n.T("checks.none") {
		t.Errorf("declined = %q, want %q: the fetch has landed, nothing is loading",
			m.declined, i18n.T("checks.none"))
	}
}

func TestRerunOnAPullRequestWithNoChecksSaysSo(t *testing.T) {
	t.Parallel()

	m := press(openChecks(t, 120, 30, domain.Checks{}), "R")
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
func appCheck() domain.Checks {
	return domain.Checks{
		Total: 1, Passed: 1, State: domain.CheckSuccess,
		Runs: []domain.CheckRun{
			{Name: "codecov/patch", State: domain.CheckSuccess, Kind: domain.CheckKindRun, Job: "7"},
		},
	}
}

// openChecks opens the list on the given checks. The height is the caller's:
// a tall screen draws every check at once, and a short one makes the list
// scroll.
func openChecks(t *testing.T, width, height int, c domain.Checks) Model {
	t.Helper()

	m := New(&fakeSource{checks: c}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: c})
	return m
}

func TestACheckWithNoWorkflowRunGetsTheOtherHeading(t *testing.T) {
	t.Parallel()

	rows := openChecks(t, 120, 30, appCheck()).allRows()
	if len(rows) != 2 {
		t.Fatalf("the list drew %d lines, want 2: a check with no workflow run gets a heading:\n%s",
			len(rows), strings.Join(rows, "\n"))
	}
	if got, want := ansi.Strip(rows[0]), i18n.T("checks.other"); got != want {
		t.Errorf("the heading reads %q, want %q", got, want)
	}
}

// The checks with no workflow run behind them -- a StatusContext, and a check
// run an App created -- used to read as the continuation of whatever group
// happened to sit above them. They share one heading, so it is drawn once.
func TestTheChecksWithNoRunOfTheirOwnGetTheirOwnHeading(t *testing.T) {
	t.Parallel()

	m := openChecks(t, 120, 30, mixed())
	want := i18n.T("checks.other")
	got := 0
	for _, line := range listPane(m) {
		if line == want {
			got++
		}
	}
	if got != 1 {
		t.Errorf("the heading %q is drawn %d times, want 1:\n%s", want, got, m.View())
	}
}

// The cursor counts checks, but the window scrolls by lines, and a heading is
// a line. On a screen too short to hold the list, a window that does not count
// the new heading stops one line above the last check and leaves it undrawn.
func TestTheCursorReachesTheLastCheckPastTheNewHeading(t *testing.T) {
	t.Parallel()

	m := openChecks(t, 80, 12, mixed())
	for range len(m.order) - 1 {
		m = press(m, "j")
	}
	pane := listPane(m)
	if last := m.order[len(m.order)-1].Name; !slices.ContainsFunc(pane, func(l string) bool {
		return strings.Contains(l, last)
	}) {
		t.Errorf("the cursor is on %q and it is not drawn:\n%s", last, m.View())
	}
	if !slices.Contains(pane, i18n.T("checks.other")) {
		t.Errorf("the heading of the group the cursor is in is off the top:\n%s", m.View())
	}
}

func names(runs []domain.CheckRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.Name
	}
	return out
}

// TestAnExternalCIsStateDoesNotMoveAnAppsCheck guards how arrange buckets a
// check with no workflow behind it. A check run an App created and a
// StatusContext both carry an empty workflow name, so one shared bucket lets
// the worst state among them rank all of them: an external CI turning green
// would move the App's check out of the middle of a workflow's group.
func TestAnExternalCIsStateDoesNotMoveAnAppsCheck(t *testing.T) {
	t.Parallel()

	succeeded := mixed()
	succeeded.Runs[8].State = domain.CheckSuccess // the StatusContext, and nothing else

	want := []string{"sca", "audit", "secrets", "deps", "build", "lint", "test", "codecov/patch", "ci/circleci"}
	for _, tc := range []struct {
		name   string
		checks domain.Checks
	}{
		{"external CI running", mixed()},
		{"external CI succeeded", succeeded},
	} {
		got := names(openChecks(t, 120, 30, tc.checks).order)
		if !slices.Equal(got, want) {
			t.Errorf("%s: order = %v, want %v", tc.name, got, want)
		}
	}
}

// listPane is the left column of the drawn view as plain text, so a workflow
// heading ("delta") can be told apart from the checks under it
// ("delta-job0"), which carry the workflow's name too.
func listPane(m Model) []string {
	var out []string
	for _, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		left, _, ok := strings.Cut(line, "│")
		if !ok {
			continue
		}
		out = append(out, strings.TrimSpace(left))
	}
	return out
}

// TestScrollingBackToAGroupsFirstCheckBringsItsHeading guards what the window
// has to hold besides the cursor's own line. Scrolling up onto the first
// check of a group leaves that group's heading one line above the window, and
// the check reads as belonging to no workflow at all.
func TestScrollingBackToAGroupsFirstCheckBringsItsHeading(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{checks: manyChecks()}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: manyChecks()})
	for range 31 {
		m = press(m, "j")
	}
	for range 7 {
		m = press(m, "k")
	}
	if got := m.order[m.row].Name; got != "delta-job0" {
		t.Fatalf("the cursor sits on %q, want the first check of the last group", got)
	}
	if !slices.Contains(listPane(m), "delta") {
		t.Errorf("the group's heading is off the top of the window:\n%s", m.View())
	}
}

// TestALongCheckNameDropsItsDurationRatherThanCutIt guards what the list's
// fixed 22 columns do when a name leaves no room for the duration. A cut
// duration ("0:…") does not read as cut off; it reads as a duration that
// starts at zero, which is a different thing from what the check took.
func TestALongCheckNameDropsItsDurationRatherThanCutIt(t *testing.T) {
	t.Parallel()

	var got string
	for _, line := range listPane(openChecks(t, 120, 30, mixed())) {
		if strings.Contains(line, "codecov/patch") {
			got = line
		}
	}
	if got == "" {
		t.Fatal("the check is not drawn at all")
	}
	if !strings.HasSuffix(got, "codecov/patch") {
		t.Errorf("the row is %q, want it to end at the name: the duration does not fit", got)
	}
}

// TestALogIsNotAskedForACheckWithNoJobBehindIt guards what enter is allowed
// to ask for. A check run an App created carries a check run id where a
// workflow's check carries an Actions job id, so asking gh for its log fails
// on an id no job has.
func TestALogIsNotAskedForACheckWithNoJobBehindIt(t *testing.T) {
	t.Parallel()

	m, cmd := openChecks(t, 120, 30, appCheck()).Update(keyPress("enter"))
	if cmd != nil {
		t.Errorf("enter started a log fetch for a check with no job behind it")
	}
	if m.declined == "" {
		t.Error("enter declined without saying why")
	}
}

func TestTheLogDoesNotWrap(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", 400)
	src := &fakeSource{checks: fixture(), log: []domain.LogLine{{Step: "s", Text: long}}}
	m := New(src, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61})
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
