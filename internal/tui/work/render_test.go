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
	if out := press(loaded(), "j").View(); !strings.Contains(out, "kukv/koto#3") {
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
	if !strings.Contains(out, "kukv/octoscope#7") {
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
}

// TestFailingChecksComeFirst is why the drawer sorts: a failure is the reason
// to look at the list at all, and the budget cuts the tail off.
func TestFailingChecksComeFirst(t *testing.T) {
	m := loaded()
	lines := m.checkLines(gh.Checks{
		Total: 3, Passed: 1, Failed: 1, Running: 1, State: gh.CheckFailure,
		Runs: []gh.CheckRun{
			{Name: "build", State: gh.CheckSuccess},
			{Name: "lint", State: gh.CheckRunning},
			{Name: "test", State: gh.CheckFailure},
		},
	})
	got := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Index(got, "test") > strings.Index(got, "build") {
		t.Errorf("the failing check is listed after a passing one:\n%s", got)
	}
}

// TestALongChecksListIsCutWithACount keeps the drawer a fixed height: it is
// drawn under the board, and a repository with thirty checks must not push
// the footer off the screen.
func TestALongChecksListIsCutWithACount(t *testing.T) {
	c := gh.Checks{Total: 12, Passed: 12, State: gh.CheckSuccess}
	for i := range 12 {
		c.Runs = append(c.Runs, gh.CheckRun{Name: fmt.Sprintf("job-%d", i), State: gh.CheckSuccess})
	}

	lines := loaded().checkLines(c)
	if want := 1 + drawerChecks + 1; len(lines) != want { // summary, the checks, the count
		t.Errorf("the list is %d lines, want %d:\n%s", len(lines), want, strings.Join(lines, "\n"))
	}
	if got := ansi.Strip(lines[len(lines)-1]); !strings.Contains(got, "7") {
		t.Errorf("the last line does not count what was left out: %q", got)
	}
}

// TestACardIsTwoLines pins spec §4.1: the title and the state marker on the
// first line, the repository, the checks bar and the elapsed time on the
// second. Three lines per card is what Phase 1 shipped.
func TestACardIsTwoLines(t *testing.T) {
	const w = 40 // wide enough that the repository is not truncated away
	now := time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)
	it := sampleWork()[gh.SectionReviewRequested][0] // a PR with failing checks

	lines := cardLines(it, w, false, now)
	if len(lines) != 2 {
		t.Fatalf("a card is %d lines, want 2:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, line := range lines {
		if got := ansi.StringWidth(line); got != w {
			t.Errorf("line %d is %d columns, want %d: %q", i+1, got, w, ansi.Strip(line))
		}
	}

	title, meta := ansi.Strip(lines[0]), ansi.Strip(lines[1])
	if !strings.Contains(title, it.Title) {
		t.Errorf("the first line does not carry the title: %q", title)
	}
	for _, want := range []string{it.Ref.Repo, "▰", "3h ago"} {
		if !strings.Contains(meta, want) {
			t.Errorf("the second line is missing %q: %q", want, meta)
		}
	}
}

// TestLabelsAreDrawnAsFilledBadges guards spec §4.5: GitHub's own label
// colour, filled, not just the name in plain text.
func TestLabelsAreDrawnAsFilledBadges(t *testing.T) {
	it := sampleWork()[gh.SectionReviewRequested][0] // carries "bug" and "ci"

	line := cardTitle(it, 60, false)
	for _, l := range it.Labels {
		if !strings.Contains(ansi.Strip(line), l.Name) {
			t.Errorf("the label %q is not on the card: %q", l.Name, ansi.Strip(line))
		}
	}
	if !strings.Contains(line, "48;2;215;58;74") {
		t.Errorf("the bug label is not filled with the colour GitHub gave it: %q", line)
	}
}

// TestABadgeIsDroppedRatherThanCut is the rule that keeps a narrow column
// readable: half a coloured label says nothing, and the title is worth more.
func TestABadgeIsDroppedRatherThanCut(t *testing.T) {
	it := sampleWork()[gh.SectionReviewRequested][0]

	for _, w := range []int{18, 22, 26} {
		line := cardTitle(it, w, false)
		if got := ansi.StringWidth(line); got != w {
			t.Errorf("width %d: the card is %d columns: %q", w, got, ansi.Strip(line))
		}
		if strings.Contains(ansi.Strip(line), "…") && strings.Contains(line, "48;2;") {
			t.Errorf("width %d: a badge was drawn onto a title that had to be cut: %q",
				w, ansi.Strip(line))
		}
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

func TestEveryRowStartsItsColumnsAtTheSameOffset(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{80, 100, 120} {
			m := New(&fakeSource{work: alignedWork()})
			m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
			m, _ = m.Update(workMsg(alignedWork()))

			colW := m.columnWidth(m.columns())
			measured := 0
			for _, line := range strings.Split(m.View(), "\n") {
				s := ansi.Strip(line)
				// The drawer repeats the selected card's tokens outside the
				// columns; it is the only line with a "#" in this fixture.
				if strings.Contains(s, "#") {
					continue
				}
				for i := range m.columns() {
					start := i * (colW + columnGap)
					// The heading and the repository row sit in the cursor
					// gutter; a title row also carries the review glyph.
					measured += checkTokenOffset(t, lang, width, s,
						i18n.T(sectionTitleIDs[gh.WorkSections()[i]]), start+2)
					measured += checkTokenOffset(t, lang, width, s, fmt.Sprintf("repo-%d", i), start+2)
					measured += checkTokenOffset(t, lang, width, s, fmt.Sprintf("title-%d", i), start+4)
				}
			}
			// One heading, one title and one repository row per column: an
			// assertion that never found its token would prove nothing.
			if want := 3 * m.columns(); measured != want {
				t.Errorf("lang %s width %d: measured %d offsets, want %d",
					lang, width, measured, want)
			}
		}
	}
}

// checkTokenOffset fails t when token appears in line at any display column
// other than want. A line without the token says nothing and is skipped; the
// return value counts the lines that did carry it.
func checkTokenOffset(t *testing.T, lang language.Tag, width int, line, token string, want int) int {
	t.Helper()
	i := strings.Index(line, token)
	if i < 0 {
		return 0
	}
	if got := ansi.StringWidth(line[:i]); got != want {
		t.Errorf("lang %s width %d: %q starts at column %d, want %d: %q",
			lang, width, token, got, want, line)
	}
	return 1
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
