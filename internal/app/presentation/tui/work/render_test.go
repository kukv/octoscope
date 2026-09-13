package work

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/i18n"
)

// overlongWork is sampleWork plus a card whose title is wider than any
// terminal the width test uses, in both scripts. Without it the fixture's
// longest line is 17 columns and every regime has room to spare, so the width
// test would pass even with the truncation removed.
func overlongWork() domain.Work {
	w := sampleWork()
	long := w[domain.SectionReviewRequested][0]
	long.Ref = domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/a-repository-with-a-name-nobody-would-choose", Number: 999}
	long.Title = "レンダリングのパイプラインをまるごと置き換える refactor that nobody asked for"
	w[domain.SectionReviewRequested] = append(w[domain.SectionReviewRequested], long)
	return w
}

// overlong returns a loaded model whose cursor sits on the overlong card, so
// the drawer renders it too.
func overlong() Model {
	m := New(&fakeSource{work: overlongWork()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = answeredAll(m, overlongWork())
	return press(press(m, "j"), "j")
}

func TestViewShowsEveryColumnHeading(t *testing.T) {
	out := loaded().View()
	for _, want := range []string{
		i18n.T("work.review_requested"),
		i18n.T("work.your_prs"),
		i18n.T("work.assigned"),
		i18n.T("work.mentioned"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view is missing the %q heading", want)
		}
	}
}

func TestViewShowsTheSelectedCardInTheDrawer(t *testing.T) {
	if out := press(loaded(), "j").View(); !strings.Contains(out, "kukv/koto #3") {
		t.Errorf("drawer does not name the selected card:\n%s", out)
	}
}

// TestTheDrawerOnlyReportsChecksForPullRequests pins the difference between a
// pull request whose checks have not run and an issue, which has no checks to
// report at all.
func TestTheDrawerOnlyReportsChecksForPullRequests(t *testing.T) {
	checkless := press(loaded(), "j") // kukv/koto#3, a PR with no checks yet
	if out := checkless.View(); !strings.Contains(out, i18n.T("work.no_checks")) {
		t.Errorf("a PR without checks does not say so:\n%s", out)
	}

	issue := press(press(loaded(), "l"), "l") // kukv/octoscope#7, in Assigned
	if _, ok := issue.SelectedRef(); !ok {
		t.Fatal("no card is selected in the Assigned column")
	}
	out := issue.View()
	if !strings.Contains(out, "kukv/octoscope #7") {
		t.Fatalf("the drawer does not show the issue:\n%s", out)
	}
	if strings.Contains(out, i18n.T("work.no_checks")) {
		t.Errorf("the drawer claims an issue has no checks:\n%s", out)
	}
}

// TestTheDrawerShowsTheBodyAndEachCheck pins spec §4.1: the drawer is what
// lets a card be read without pressing enter, so it carries the body and the
// checks one by one — not a summary line.
func TestTheDrawerShowsTheBodyAndEachCheck(t *testing.T) {
	out := ansi.Strip(loaded().View())

	if !strings.Contains(out, "The renderer dropped every escape.") {
		t.Errorf("the drawer does not show the body:\n%s", out)
	}
	for _, name := range []string{"build", "lint", "test"} {
		if !strings.Contains(out, name) {
			t.Errorf("the drawer does not name the %q check:\n%s", name, out)
		}
	}
	// The fixture's body uses CRLF, as a body written on Windows does. A
	// carriage return left in a drawn line sends the terminal's cursor back to
	// the start of it and shifts everything after it.
	if strings.Contains(out, "\r") {
		t.Errorf("a carriage return survived into the drawn board: %q", out)
	}
}

// TestFailingChecksComeFirst is why the drawer sorts: a failure is the reason
// to look at the list at all, and the budget cuts the tail off.
func TestFailingChecksComeFirst(t *testing.T) {
	m := loaded()
	lines := m.checksPane(domain.WorkItem{
		Ref: domain.ItemRef{Kind: domain.ItemPR},
		Checks: domain.Checks{
			Total: 3, Passed: 1, Failed: 1, Running: 1, State: domain.CheckFailure,
			Runs: []domain.CheckRun{
				{Name: "build", State: domain.CheckSuccess},
				{Name: "lint", State: domain.CheckRunning},
				{Name: "test", State: domain.CheckFailure},
			},
		},
	}, 40)
	got := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Index(got, "test") > strings.Index(got, "build") {
		t.Errorf("the failing check is listed after a passing one:\n%s", got)
	}
}

// TestALongChecksListIsCutWithACount keeps the drawer a fixed height: it is
// drawn under the board, and a repository with thirty checks must not push
// the key bar off the screen.
func TestALongChecksListIsCutWithACount(t *testing.T) {
	c := domain.Checks{Total: 12, Passed: 12, State: domain.CheckSuccess}
	for i := range 12 {
		c.Runs = append(c.Runs, domain.CheckRun{Name: fmt.Sprintf("job-%d", i), State: domain.CheckSuccess})
	}

	lines := loaded().checksPane(domain.WorkItem{Ref: domain.ItemRef{Kind: domain.ItemPR}, Checks: c}, 40)
	if want := drawerChecks + 2; len(lines) != want { // the checks, the count, the summary
		t.Errorf("the list is %d lines, want %d:\n%s", len(lines), want, strings.Join(lines, "\n"))
	}
	if got := ansi.Strip(lines[drawerChecks]); !strings.Contains(got, "9") {
		t.Errorf("the line after the list does not count what was left out: %q", got)
	}
}

// TestTheDrawerIsAlwaysTheSameHeight is what keeps the key bar still: the
// drawer sits under a board whose length depends on the data, and a drawer
// that grew with its contents would move everything below it.
func TestTheDrawerIsAlwaysTheSameHeight(t *testing.T) {
	for name, m := range map[string]Model{
		"a PR with checks and a body": loaded(),
		"a PR with neither":           press(loaded(), "j"),
		"an issue":                    press(press(loaded(), "l"), "l"),
		"an empty column":             press(loaded(), "l"),
	} {
		if got := len(m.drawer()); got != drawerHeight {
			t.Errorf("%s: the drawer is %d lines, want %d", name, got, drawerHeight)
		}
	}
}

// TestTheDrawerNamesTheBranchesAndTheSizeOfTheChange is the meta line the
// mockup puts under the title.
func TestTheDrawerNamesTheBranchesAndTheSizeOfTheChange(t *testing.T) {
	out := ansi.Strip(strings.Join(loaded().drawer(), "\n"))
	for _, want := range []string{"kukv/octoscope #12", "feat/graph", "main", "+218", "−31"} {
		if !strings.Contains(out, want) {
			t.Errorf("the drawer is missing %q:\n%s", want, out)
		}
	}
}

// boardClock is when boardOf's columns answered. The cards show relative
// times, so a test that reads one needs a fixed clock rather than the wall.
var boardClock = time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)

// boardOf is a loaded board at one width, for the tests that ask a single
// piece of the drawing what it produced.
func boardOf(width int) Model {
	m := New(&fakeSource{work: sampleWork()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m = answeredAll(m, sampleWork())
	for _, s := range domain.WorkSections() {
		m.fetchedAt[s] = boardClock
	}
	return m
}

// TestABoxedCardIsSixLines pins the shape the mockup draws: the head line,
// the title over two lines and the meta line under it, inside a box of its
// own.
func TestABoxedCardIsSixLines(t *testing.T) {
	const w = 34
	m := boardOf(160)
	it := sampleWork()[domain.SectionReviewRequested][0] // a PR with failing checks

	lines := m.card(it, boardClock, w, false)
	if len(lines) != m.cardHeight() || len(lines) != titleLines+4 {
		t.Fatalf("a boxed card is %d lines, want %d:\n%s",
			len(lines), titleLines+4, strings.Join(lines, "\n"))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != w {
			t.Errorf("line %d is %d columns, want %d: %q", i+1, got, w, ansi.Strip(line))
		}
	}
	last := len(lines) - 1
	if !strings.HasPrefix(ansi.Strip(lines[0]), "╭") || !strings.HasPrefix(ansi.Strip(lines[last]), "╰") {
		t.Errorf("the card has no box:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	}
	if head := ansi.Strip(lines[1]); !strings.Contains(head, "#12") {
		t.Errorf("the first line inside the box wants the number: %q", head)
	}
	if title := ansi.Strip(lines[2]); !strings.Contains(title, it.Title) {
		t.Errorf("the line under the head wants the title: %q", title)
	}
}

// TestANarrowCardLosesItsBox is the degradation step the boxes forced: below
// a hundred columns the drawer and the fourth column go, and a border would
// cost two lines of every card on a screen that is usually short too.
func TestANarrowCardLosesItsBox(t *testing.T) {
	const w = 38
	m := boardOf(80)
	it := sampleWork()[domain.SectionReviewRequested][0]

	lines := m.card(it, boardClock, w, false)
	if len(lines) != m.cardHeight() || len(lines) != titleLines+2 {
		t.Fatalf("an unboxed card is %d lines, want %d:\n%s",
			len(lines), titleLines+2, strings.Join(lines, "\n"))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != w {
			t.Errorf("line %d is %d columns, want %d: %q", i+1, got, w, ansi.Strip(line))
		}
	}
	if strings.Contains(ansi.Strip(lines[0]), "╭") {
		t.Error("the card still has a box below a hundred columns")
	}
}

// TestTheCardHeadNamesTheRepositoryInFull is the first line of a card: which
// repository, and which number in it. The owner is part of the answer — the
// board gathers work from every repository the user touches, and half of them
// are not theirs.
func TestTheCardHeadNamesTheRepositoryInFull(t *testing.T) {
	m := boardOf(80)
	it := sampleWork()[domain.SectionReviewRequested][0] // kukv/octoscope#12

	head := ansi.Strip(m.cardHead(it, 36, false, gutter))
	if !strings.Contains(head, "kukv/octoscope") {
		t.Errorf("the owner is missing from the first line: %q", head)
	}
	if !strings.Contains(head, "#12") {
		t.Errorf("the number is missing from the first line: %q", head)
	}
}

// TestANarrowCardHeadKeepsTheNumberWhole is what the first line gives up
// first. The number is what identifies the card; half a repository name is
// still a hint, half a number is nothing.
func TestANarrowCardHeadKeepsTheNumberWhole(t *testing.T) {
	m := boardOf(120)
	it := domain.WorkItem{
		Ref: domain.ItemRef{
			Kind: domain.ItemPR, Repo: "kukv/a-repository-with-a-name-nobody-would-choose", Number: 999,
		},
	}

	const w = 20
	if full := ansi.Strip(m.cardHead(it, 100, false, "")); ansi.StringWidth(full) <= w {
		t.Fatalf("the head fits in %d columns uncut; this test covers nothing: %q", w, full)
	}
	head := ansi.Strip(m.cardHead(it, w, false, ""))
	if got := ansi.StringWidth(head); got > w {
		t.Errorf("the head is %d columns, want at most %d: %q", got, w, head)
	}
	if !strings.Contains(head, "#999") {
		t.Errorf("the number did not survive the clip: %q", head)
	}
	if !strings.Contains(head, "…") {
		t.Errorf("nothing was clipped, so the number survived by luck: %q", head)
	}
}

// TestTheTitleLinesCarryOnlyTheTitle pins what moved to the head line: a
// title line that still spelled the number would spend the columns twice.
func TestTheTitleLinesCarryOnlyTheTitle(t *testing.T) {
	m := boardOf(80)
	it := sampleWork()[domain.SectionReviewRequested][0]

	for i, line := range m.cardTitle(it, 36, false, gutter) {
		got := ansi.Strip(line)
		if strings.Contains(got, "#12") || strings.Contains(got, "octoscope") {
			t.Errorf("title line %d repeats the head: %q", i+1, got)
		}
	}
}

// TestTheCardMetaIsTheBarThenTheAgeThenTheLabels pins the order of the last
// line. The repository left it for the head line.
func TestTheCardMetaIsTheBarThenTheAgeThenTheLabels(t *testing.T) {
	m := boardOf(160)
	it := sampleWork()[domain.SectionReviewRequested][0] // failing checks, two labels

	meta := ansi.Strip(m.cardMeta(it, boardClock, 60))
	if strings.Contains(meta, "octoscope") {
		t.Errorf("the repository is still on the meta line: %q", meta)
	}
	bar, age, label := strings.Index(meta, "▰"), strings.Index(meta, "3h ago"), strings.Index(meta, "bug")
	if bar < 0 || age < 0 || label < 0 {
		t.Fatalf("the meta line is missing the bar, the age or a label: %q", meta)
	}
	if bar >= age || age >= label {
		t.Errorf("the meta line reads %q, want the bar, then the age, then the labels", meta)
	}
}

// TestALongTitleRunsOntoTheSecondLine is why the card grew a line: a title
// cut at the width of one narrow column says nothing about the pull request.
func TestALongTitleRunsOntoTheSecondLine(t *testing.T) {
	m := boardOf(80)
	it := domain.WorkItem{
		Ref:   domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12},
		Title: "replace the whole rendering pipeline with something readable",
	}

	lines := m.cardTitle(it, 36, false, gutter)
	if len(lines) != titleLines {
		t.Fatalf("the title is %d lines, want %d", len(lines), titleLines)
	}
	first, second := ansi.Strip(lines[0]), strings.TrimSpace(ansi.Strip(lines[1]))
	if !strings.Contains(first, "replace") {
		t.Errorf("the first line carries no title: %q", first)
	}
	if second == "" {
		t.Errorf("the second line is empty; the title was cut instead of wrapped: %q", first)
	}
	if strings.Contains(second, "replace") {
		t.Errorf("the second line repeats the first: %q", second)
	}
}

// TestAWrappedTitleLosesNothing is the assertion the width checks cannot
// make: a wrap that joined the pieces back with a space, or one whose second
// line started from the top of the title again, still fits the column.
func TestAWrappedTitleLosesNothing(t *testing.T) {
	m := boardOf(80)
	for name, tc := range map[string]struct{ title, sep string }{
		// English wraps on a space, and that one space is consumed by the
		// break. Japanese has no space to break on, so nothing is consumed
		// and nothing may be invented either.
		"english":  {"replace the whole rendering pipeline", " "},
		"japanese": {"レンダリングのパイプラインをまるごと置き換える", ""},
	} {
		it := domain.WorkItem{
			Ref:   domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12},
			Title: tc.title,
		}
		lines := m.cardTitle(it, 36, false, gutter)
		first := strings.TrimPrefix(ansi.Strip(lines[0]), gutter)
		second := strings.TrimPrefix(ansi.Strip(lines[1]), gutter)
		if got := first + tc.sep + second; got != tc.title {
			t.Errorf("%s: the two lines read %q, want the whole title %q", name, got, tc.title)
		}
	}
}

// TestAWrappedJapaneseTitleGainsNoSpaces is the case the two-line fixtures
// above cannot reach: a title long enough to wrap three times. Folding the
// leftover lines back together with a space would put spaces into a title
// whose author wrote none, and only a title that overflows twice shows it.
func TestAWrappedJapaneseTitleGainsNoSpaces(t *testing.T) {
	const title = "レンダリングのパイプラインをまるごと置き換えるための大きな変更"
	m := boardOf(80)
	it := domain.WorkItem{
		Ref:   domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12},
		Title: title,
	}

	lines := m.cardTitle(it, 36, false, gutter)
	if n := len(strings.Split(ansi.Wrap(title, 28, ""), "\n")); n < 3 {
		t.Fatalf("the title wraps to %d lines; this test covers nothing", n)
	}
	second := strings.TrimPrefix(ansi.Strip(lines[1]), gutter)
	if strings.Contains(second, " ") {
		t.Errorf("the second line gained a space the title never had: %q", second)
	}
	if !strings.Contains(title, strings.TrimSuffix(second, "…")) {
		t.Errorf("the second line is not part of the title: %q", second)
	}
}

// TestAShortTitleStillFillsTheCard keeps the card a fixed height: visibleCards,
// cardWindow and the mouse hit-test all divide by it, and a card that shrank
// with its title would put them out by however many short titles sat above.
func TestAShortTitleStillFillsTheCard(t *testing.T) {
	it := domain.WorkItem{
		Ref:   domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/koto", Number: 3},
		Title: "docs",
	}
	for width, w := range map[int]int{80: 38, 160: 34} {
		m := boardOf(width)
		if got := len(m.card(it, boardClock, w, false)); got != m.cardHeight() {
			t.Errorf("width %d: a short title makes a %d-line card, want %d",
				width, got, m.cardHeight())
		}
	}
}

// TestAShortBoardStillDrawsACard covers the floor under the height budget: a
// terminal too short for one taller card must still show one. The board is
// asked directly rather than the whole view, because the drawer repeats the
// selected card and would answer for it.
func TestAShortBoardStillDrawsACard(t *testing.T) {
	m := loaded()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 10})
	if m.height-footerHeight-drawerHeight >= headingHeight+m.cardHeight() {
		t.Fatal("ten lines leave room for a card; this test covers nothing")
	}

	board := ansi.Strip(strings.Join(m.board(m.boardHeight()), "\n"))
	if !strings.Contains(board, "#12") {
		t.Errorf("a ten-line terminal draws no card:\n%s", board)
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if got := ansi.StringWidth(line); got > 120 {
			t.Errorf("a line is %d columns: %q", got, ansi.Strip(line))
		}
	}
}

// TestACardIsDatedByItsOwnColumnsAnswer covers the clock the ages are
// measured against: the columns answer at very different speeds, and a card
// dated by some other column's answer would claim an age nothing on screen
// has.
func TestACardIsDatedByItsOwnColumnsAnswer(t *testing.T) {
	m := New(&fakeSource{work: sampleWork()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = answeredAll(m, sampleWork())
	for _, s := range domain.WorkSections() {
		m.fetchedAt[s] = boardClock
	}
	m.fetchedAt[domain.SectionAssigned] = boardClock.Add(48 * time.Hour)

	it := sampleWork()[domain.SectionAssigned][0]
	own := i18n.RelTime(boardClock.Add(48*time.Hour), it.UpdatedAt)
	other := i18n.RelTime(boardClock, it.UpdatedAt)
	if own == other {
		t.Fatal("the two clocks produce the same age; this test covers nothing")
	}

	column := ansi.Strip(strings.Join(m.columnLines(domain.SectionAssigned, 40, 0), "\n"))
	if !strings.Contains(column, own) {
		t.Errorf("the card is not dated %q:\n%s", own, column)
	}
	if strings.Contains(column, other) {
		t.Errorf("the card carries another column's age %q:\n%s", other, column)
	}
}

// TestAPullRequestWithoutChecksSaysWhereItsReviewStands is what the mockup
// puts where the bar would be: a card with nothing running still has to say
// something about itself.
func TestAPullRequestWithoutChecksSaysWhereItsReviewStands(t *testing.T) {
	m := boardOf(160)

	approved := domain.WorkItem{
		Ref:   domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 43},
		Title: "docs", Review: domain.ReviewApproved,
	}
	if got := ansi.Strip(m.cardMeta(approved, boardClock, 60)); !strings.Contains(got, i18n.T("review.approved")) {
		t.Errorf("an approved PR with no checks says nothing: %q", got)
	}

	draft := approved
	draft.IsDraft = true
	if got := ansi.Strip(m.cardMeta(draft, boardClock, 60)); !strings.Contains(got, i18n.T("work.draft")) {
		t.Errorf("a draft does not say so: %q", got)
	}

	issue := domain.WorkItem{Ref: domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/koto", Number: 8}, Title: "an issue"}
	got := ansi.Strip(m.cardMeta(issue, boardClock, 60))
	if strings.Contains(got, i18n.T("review.approved")) || strings.Contains(got, i18n.T("work.draft")) {
		t.Errorf("an issue was given a review word: %q", got)
	}
}

// TestLabelsAreDrawnAsFilledBadges guards spec 4.5: GitHub's own label
// colour, filled, not just the name in plain text. The mockup puts them on
// the meta line, beside the repository.
func TestLabelsAreDrawnAsFilledBadges(t *testing.T) {
	m := boardOf(160)
	it := sampleWork()[domain.SectionReviewRequested][0] // carries "bug" and "ci"

	line := m.cardMeta(it, boardClock, 60)
	for _, l := range it.Labels {
		if !strings.Contains(ansi.Strip(line), l.Name) {
			t.Errorf("the label %q is not on the card: %q", l.Name, ansi.Strip(line))
		}
	}
	if !strings.Contains(line, "48;2;215;58;74") {
		t.Errorf("the bug label is not filled with the colour GitHub gave it: %q", line)
	}
}

// TestTheColumnHeadingCountsWhatIsInIt is the point of the board: how much
// has piled up has to be readable even when the column is scrolled.
func TestTheColumnHeadingCountsWhatIsInIt(t *testing.T) {
	m := boardOf(160)

	head := ansi.Strip(m.heading(domain.SectionReviewRequested, 3, 30))
	if !strings.HasSuffix(strings.TrimRight(head, " "), "3") {
		t.Errorf("the count is not at the end of the heading: %q", head)
	}
	if got := ansi.StringWidth(m.heading(domain.SectionReviewRequested, 3, 30)); got != 30 {
		t.Errorf("the heading is %d columns, want 30", got)
	}
	// A column with nothing in it shows no count rather than a zero.
	if empty := ansi.Strip(m.heading(domain.SectionYourPRs, 0, 30)); strings.Contains(empty, "0") {
		t.Errorf("an empty column is counted: %q", empty)
	}
	// Review requested is the column that wants attention, and says so.
	if m.heading(domain.SectionReviewRequested, 3, 30) == m.heading(domain.SectionAssigned, 3, 30) {
		t.Error("a waiting review is coloured like anything else")
	}
}

// TestTheChecksBarIsColouredByOutcome is why icon.ChecksBar hands back its two
// halves apart: a bar drawn in one colour says nothing about whether the
// checks are passing.
func TestTheChecksBarIsColouredByOutcome(t *testing.T) {
	failing := checksBar(domain.Checks{Total: 4, Passed: 2, Failed: 2, State: domain.CheckFailure})
	passing := checksBar(domain.Checks{Total: 4, Passed: 4, State: domain.CheckSuccess})
	if failing == passing {
		t.Errorf("a failing bar looks like a passing one: %q", failing)
	}
	if got := checksBar(domain.Checks{}); got != "" {
		t.Errorf("a card with no checks still draws a bar: %q", got)
	}
}

func TestEmptyColumnSaysSo(t *testing.T) {
	if out := loaded().View(); !strings.Contains(out, i18n.T("work.empty_column")) {
		t.Errorf("no empty-column marker for Your PRs:\n%s", out)
	}
}

func TestNarrowTerminalDropsTheDrawer(t *testing.T) {
	m := loaded()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if strings.Contains(m.View(), "kukv/octoscope#12") {
		t.Error("the drawer is still drawn at 80 columns")
	}
}

func TestVeryNarrowTerminalShowsOneColumn(t *testing.T) {
	m := loaded()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 40})
	out := m.View()
	if strings.Contains(out, i18n.T("work.mentioned")) {
		t.Error("all four headings are drawn at 50 columns")
	}
	if !strings.Contains(out, i18n.T("work.review_requested")) {
		t.Error("the current column's heading is missing")
	}
	if !strings.Contains(out, i18n.Tf("work.column_position", map[string]any{"Index": 1, "Total": 4})) {
		t.Errorf("the single column does not say which column it is:\n%s", out)
	}
}

// headingTexts is every column heading, in the order the columns are drawn.
func headingTexts() []string {
	texts := make([]string, 0, len(domain.WorkSections()))
	for _, s := range domain.WorkSections() {
		texts = append(texts, i18n.T(sectionTitleIDs[s]))
	}
	return texts
}

// headingsOn reports which column headings a board drew, by name.
func headingsOn(m Model) []string {
	out := ansi.Strip(m.View())
	var drawn []string
	for _, text := range headingTexts() {
		if strings.Contains(out, text) {
			drawn = append(drawn, text)
		}
	}
	return drawn
}

// TestTheHeadingsAreAllDifferent is what the two tests below rest on: they
// count headings to count columns, which says nothing if two columns are
// named the same.
func TestTheHeadingsAreAllDifferent(t *testing.T) {
	seen := map[string]bool{}
	for _, text := range headingTexts() {
		if text == "" || seen[text] {
			t.Fatalf("the column headings are not distinct: %q", headingTexts())
		}
		seen[text] = true
	}
}

// TestHowManyColumnsFitTheWidth pins the three tiers. Four columns at eighty
// leave seven columns for a title, which says nothing about the pull request;
// two columns leave twenty-eight.
func TestHowManyColumnsFitTheWidth(t *testing.T) {
	for _, tc := range []struct{ width, want int }{
		{50, 1}, {59, 1}, {60, 2}, {80, 2}, {99, 2}, {100, 4}, {160, 4},
	} {
		m := loaded()
		m, _ = m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 40})
		if got := len(headingsOn(m)); got != tc.want {
			t.Errorf("width %d draws %d columns, want %d:\n%s",
				tc.width, got, tc.want, ansi.Strip(m.View()))
		}
	}
}

// TestTwoColumnsPageByWholePages is why the tiers are one, two and four: the
// four sections divide by each of them, so h and l move between pages that
// are full rather than leaving a page with one column in it.
func TestTwoColumnsPageByWholePages(t *testing.T) {
	texts := headingTexts()
	for col, want := range map[int][]string{
		0: {texts[0], texts[1]},
		1: {texts[0], texts[1]},
		2: {texts[2], texts[3]},
		3: {texts[2], texts[3]},
	} {
		m := loaded()
		m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
		m.col = col
		got := headingsOn(m)
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("column %d shows %q, want %q", col, got, want)
		}
	}
}

// TestAPagedBoardSaysWhichColumnTheCursorIsIn covers both paged tiers: the
// count is out of four either way, because that is how many columns the user
// can reach.
func TestAPagedBoardSaysWhichColumnTheCursorIsIn(t *testing.T) {
	for _, width := range []int{50, 80} {
		m := loaded()
		m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		m.col = 2
		want := i18n.Tf("work.column_position", map[string]any{"Index": 3, "Total": 4})
		if !strings.Contains(m.View(), want) {
			t.Errorf("width %d does not say which column the cursor is in:\n%s", width, m.View())
		}
	}
}

func TestLoadingBoardSaysSo(t *testing.T) {
	m := New(&fakeSource{work: sampleWork()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Refresh()
	t.Cleanup(m.Cancel)
	out := m.View()
	if !strings.Contains(out, i18n.T("common.loading")) {
		t.Errorf("a loading board does not say so:\n%s", out)
	}
	// The board draws its own spinner, as the repo list and the detail view
	// do; "⣾" is the first frame of spinner.Dot.
	if !strings.Contains(out, "⣾") {
		t.Errorf("a loading board does not animate:\n%s", out)
	}
}

func TestSpinnerTickAdvancesTheFrame(t *testing.T) {
	m := New(&fakeSource{work: sampleWork()})
	before := m.spin.View()
	m, cmd := m.Update(m.spin.Tick())
	if cmd == nil {
		t.Fatal("a tick produced no follow-up command; the animation would stop")
	}
	if m.spin.View() == before {
		t.Errorf("the spinner frame did not advance: still %q", before)
	}
}

func TestUnsizedBoardRendersNothing(t *testing.T) {
	if out := New(&fakeSource{}).View(); out != "" {
		t.Errorf("got %q, want an empty string before the first WindowSizeMsg", out)
	}
}

func TestNoLineExceedsTheTerminalWidth(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{50, 80, 100, 120} {
			for name, base := range map[string]func() Model{"sample": loaded, "overlong": overlong} {
				m := base()
				m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
				for _, line := range strings.Split(m.View(), "\n") {
					if w := ansi.StringWidth(line); w > width {
						t.Errorf("%s lang %s width %d: line is %d columns: %q",
							name, lang, width, w, line)
					}
				}
			}
		}
	}
}

// alignedWork gives every column one card carrying tokens that appear nowhere
// else, so the alignment test can measure where each column actually starts
// instead of trusting the padding that produced it.
func alignedWork() domain.Work {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	var w domain.Work
	for i, s := range domain.WorkSections() {
		w[s] = []domain.WorkItem{{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Repo: fmt.Sprintf("repo-%d", i), Number: i},
			Title:     fmt.Sprintf("title-%d", i),
			UpdatedAt: now,
		}}
	}
	return w
}

// TestEveryRowStartsItsColumnsAtTheSameOffset measures where each column
// actually drew its own token and asks whether the columns on screen agree, rather than
// hard-coding an offset the drawing would have to be read to know. Japanese
// takes two columns per character, so a column that measured its padding in
// runes lines up in English and drifts in Japanese.
func TestEveryRowStartsItsColumnsAtTheSameOffset(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{80, 100, 120} {
			m := New(&fakeSource{work: alignedWork()})
			m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
			m = answeredAll(m, alignedWork())

			colW := m.columnWidth(m.columnsFor())
			for _, token := range []string{"title-%d", "repo-%d"} {
				indent := -1
				for i := range m.columnsFor() {
					x, ok := offsetOf(m.board(m.boardHeight()), fmt.Sprintf(token, i))
					if !ok {
						t.Errorf("lang %s width %d: %q was never drawn", lang, width, token)
						continue
					}
					got := x - i*(colW+columnGap)
					if indent < 0 {
						indent = got
					}
					if got != indent {
						t.Errorf("lang %s width %d: %q sits %d columns into its column, want %d",
							lang, width, fmt.Sprintf(token, i), got, indent)
					}
				}
			}
		}
	}
}

// offsetOf reports the display column a token was drawn at. It is given the
// board alone: the drawer repeats the selected card's tokens outside the
// columns, where an offset means nothing.
func offsetOf(board []string, token string) (int, bool) {
	for _, line := range board {
		s := ansi.Strip(line)
		if i := strings.Index(s, token); i >= 0 {
			return ansi.StringWidth(s[:i]), true
		}
	}
	return 0, false
}

// TestKeyBarNamesTheChecksKey pins s alongside d in the board's key bar: a
// key with no hint in the footer is a key nobody can find.
func TestKeyBarNamesTheChecksKey(t *testing.T) {
	if !strings.Contains(loaded().View(), "s:checks") {
		t.Errorf("key bar = %q, want it to mention s:checks", loaded().View())
	}
}

func TestNoUnresolvedIDsInTheWorkView(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		i18n.AssertNoUnresolvedIDs(t, loaded().View())

		empty := New(&fakeSource{})
		empty, _ = empty.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
		i18n.AssertNoUnresolvedIDs(t, empty.View())

		loading, _ := empty.Refresh()
		t.Cleanup(loading.Cancel)
		i18n.AssertNoUnresolvedIDs(t, loading.View())
	}
}
