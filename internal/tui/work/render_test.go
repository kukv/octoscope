package work

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// overlongWork is sampleWork plus a card whose title is wider than any
// terminal the width test uses, in both scripts. Without it the fixture's
// longest line is 17 columns and every regime has room to spare, so the width
// test would pass even with the truncation removed.
func overlongWork() gh.Work {
	w := sampleWork()
	long := w[gh.SectionReviewRequested][0]
	long.Ref = gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/a-repository-with-a-name-nobody-would-choose", Number: 999}
	long.Title = "レンダリングのパイプラインをまるごと置き換える refactor that nobody asked for"
	w[gh.SectionReviewRequested] = append(w[gh.SectionReviewRequested], long)
	return w
}

// overlong returns a loaded model whose cursor sits on the overlong card, so
// the drawer renders it too.
func overlong() Model {
	m := New(&fakeSource{work: overlongWork()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = m.Update(workMsg(overlongWork()))
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
	lines := m.checksPane(gh.WorkItem{
		Ref: gh.ItemRef{Kind: gh.ItemPR},
		Checks: gh.Checks{
			Total: 3, Passed: 1, Failed: 1, Running: 1, State: gh.CheckFailure,
			Runs: []gh.CheckRun{
				{Name: "build", State: gh.CheckSuccess},
				{Name: "lint", State: gh.CheckRunning},
				{Name: "test", State: gh.CheckFailure},
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
	c := gh.Checks{Total: 12, Passed: 12, State: gh.CheckSuccess}
	for i := range 12 {
		c.Runs = append(c.Runs, gh.CheckRun{Name: fmt.Sprintf("job-%d", i), State: gh.CheckSuccess})
	}

	lines := loaded().checksPane(gh.WorkItem{Ref: gh.ItemRef{Kind: gh.ItemPR}, Checks: c}, 40)
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

// boardOf is a loaded board at one width, for the tests that ask a single
// piece of the drawing what it produced.
func boardOf(width int) Model {
	m := New(&fakeSource{work: sampleWork()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(workMsg(sampleWork()))
	m.fetchedAt = time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)
	return m
}

// TestABoxedCardIsFourLines pins the shape the mockup draws: two lines of
// text inside a box of its own.
func TestABoxedCardIsFourLines(t *testing.T) {
	const w = 34
	m := boardOf(160)
	it := sampleWork()[gh.SectionReviewRequested][0] // a PR with failing checks

	lines := m.card(it, w, false)
	if len(lines) != m.cardHeight() || len(lines) != 4 {
		t.Fatalf("a boxed card is %d lines, want 4:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != w {
			t.Errorf("line %d is %d columns, want %d: %q", i+1, got, w, ansi.Strip(line))
		}
	}
	if !strings.HasPrefix(ansi.Strip(lines[0]), "╭") || !strings.HasPrefix(ansi.Strip(lines[3]), "╰") {
		t.Errorf("the card has no box:\n%s", ansi.Strip(strings.Join(lines, "\n")))
	}
	if title := ansi.Strip(lines[1]); !strings.Contains(title, "#12") ||
		!strings.Contains(title, it.Title) {
		t.Errorf("the first line wants the number and the title: %q", title)
	}
}

// TestANarrowCardLosesItsBox is the degradation step the boxes forced: at
// eighty columns a column is seventeen wide, and a border would take two of
// them (spec 4.6).
func TestANarrowCardLosesItsBox(t *testing.T) {
	const w = 17
	m := boardOf(80)
	it := sampleWork()[gh.SectionReviewRequested][0]

	lines := m.card(it, w, false)
	if len(lines) != m.cardHeight() || len(lines) != 2 {
		t.Fatalf("an unboxed card is %d lines, want 2:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != w {
			t.Errorf("line %d is %d columns, want %d: %q", i+1, got, w, ansi.Strip(line))
		}
	}
	if strings.Contains(ansi.Strip(lines[0]), "╭") {
		t.Error("the card still has a box at seventeen columns")
	}
}

// TestTheCardMetaNamesTheRepositoryWithoutItsOwner keeps the second line
// readable in a column thirty wide: the owner is the same for most of them,
// and the drawer gives the full reference anyway.
func TestTheCardMetaNamesTheRepositoryWithoutItsOwner(t *testing.T) {
	m := boardOf(160)
	it := sampleWork()[gh.SectionReviewRequested][0] // kukv/octoscope

	meta := ansi.Strip(m.cardMeta(it, 60))
	if !strings.Contains(meta, "octoscope") {
		t.Errorf("the repository is missing: %q", meta)
	}
	if strings.Contains(meta, "kukv/") {
		t.Errorf("the owner is still on the card: %q", meta)
	}
	for _, want := range []string{"▰", "3h ago"} {
		if !strings.Contains(meta, want) {
			t.Errorf("the meta line is missing %q: %q", want, meta)
		}
	}
}

// TestAPullRequestWithoutChecksSaysWhereItsReviewStands is what the mockup
// puts where the bar would be: a card with nothing running still has to say
// something about itself.
func TestAPullRequestWithoutChecksSaysWhereItsReviewStands(t *testing.T) {
	m := boardOf(160)

	approved := gh.WorkItem{
		Ref:   gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 43},
		Title: "docs", Review: gh.ReviewApproved,
	}
	if got := ansi.Strip(m.cardMeta(approved, 60)); !strings.Contains(got, i18n.T("review.approved")) {
		t.Errorf("an approved PR with no checks says nothing: %q", got)
	}

	draft := approved
	draft.IsDraft = true
	if got := ansi.Strip(m.cardMeta(draft, 60)); !strings.Contains(got, i18n.T("work.draft")) {
		t.Errorf("a draft does not say so: %q", got)
	}

	issue := gh.WorkItem{Ref: gh.ItemRef{Kind: gh.ItemIssue, Repo: "kukv/koto", Number: 8}, Title: "an issue"}
	got := ansi.Strip(m.cardMeta(issue, 60))
	if strings.Contains(got, i18n.T("review.approved")) || strings.Contains(got, i18n.T("work.draft")) {
		t.Errorf("an issue was given a review word: %q", got)
	}
}

// TestLabelsAreDrawnAsFilledBadges guards spec 4.5: GitHub's own label
// colour, filled, not just the name in plain text. The mockup puts them on
// the meta line, beside the repository.
func TestLabelsAreDrawnAsFilledBadges(t *testing.T) {
	m := boardOf(160)
	it := sampleWork()[gh.SectionReviewRequested][0] // carries "bug" and "ci"

	line := m.cardMeta(it, 60)
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

	head := ansi.Strip(m.heading(gh.SectionReviewRequested, 3, 30))
	if !strings.HasSuffix(strings.TrimRight(head, " "), "3") {
		t.Errorf("the count is not at the end of the heading: %q", head)
	}
	if got := ansi.StringWidth(m.heading(gh.SectionReviewRequested, 3, 30)); got != 30 {
		t.Errorf("the heading is %d columns, want 30", got)
	}
	// A column with nothing in it shows no count rather than a zero.
	if empty := ansi.Strip(m.heading(gh.SectionYourPRs, 0, 30)); strings.Contains(empty, "0") {
		t.Errorf("an empty column is counted: %q", empty)
	}
	// Review requested is the column that wants attention, and says so.
	if m.heading(gh.SectionReviewRequested, 3, 30) == m.heading(gh.SectionAssigned, 3, 30) {
		t.Error("a waiting review is coloured like anything else")
	}
}

// TestTheChecksBarIsColouredByOutcome is why icon.ChecksBar hands back its two
// halves apart: a bar drawn in one colour says nothing about whether the
// checks are passing.
func TestTheChecksBarIsColouredByOutcome(t *testing.T) {
	failing := checksBar(gh.Checks{Total: 4, Passed: 2, Failed: 2, State: gh.CheckFailure})
	passing := checksBar(gh.Checks{Total: 4, Passed: 4, State: gh.CheckSuccess})
	if failing == passing {
		t.Errorf("a failing bar looks like a passing one: %q", failing)
	}
	if got := checksBar(gh.Checks{}); got != "" {
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
func alignedWork() gh.Work {
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	var w gh.Work
	for i, s := range gh.WorkSections() {
		w[s] = []gh.WorkItem{{
			Ref:       gh.ItemRef{Kind: gh.ItemPR, Repo: fmt.Sprintf("repo-%d", i), Number: i},
			Title:     fmt.Sprintf("title-%d", i),
			UpdatedAt: now,
		}}
	}
	return w
}

// TestEveryRowStartsItsColumnsAtTheSameOffset measures where each column
// actually drew its own token and asks whether the four agree, rather than
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
			m, _ = m.Update(workMsg(alignedWork()))

			colW := m.columnWidth(m.columns())
			for _, token := range []string{"title-%d", "repo-%d"} {
				indent := -1
				for i := range m.columns() {
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
