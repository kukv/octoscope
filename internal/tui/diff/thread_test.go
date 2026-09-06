package diff

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

func threadFixture() gh.ReviewContext {
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	return gh.ReviewContext{
		PullRequestID: "PR_1",
		Threads: []gh.ReviewThread{
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideRight,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "kukv"}, Body: "is 2 not the default?", CreatedAt: at},
				},
			},
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideLeft, Resolved: true,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "someone"}, Body: "settled long ago", CreatedAt: at},
				},
			},
			{
				Path: "graph/walk.go", Line: 900, Side: gh.SideRight,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "someone"}, Body: "on a line not in this diff", CreatedAt: at},
				},
			},
		},
	}
}

// withThreads loads the fixture at the size every caller here needs the same
// (120 columns, wide enough for a thread line and its author not to be
// truncated, and 40 rows so every fixture thread fits on screen at once).
func withThreads(t *testing.T, width, height int) Model {
	t.Helper()
	m := loaded(t, width, height)
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: threadFixture()})
	return m
}

// pendingFixture is one comment the viewer has written but not submitted, so
// the "not sent yet" marker and the header's pending count have something to
// draw.
func pendingFixture() gh.ReviewContext {
	return gh.ReviewContext{
		PullRequestID: "PR_1",
		Threads: []gh.ReviewThread{
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideRight,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "kukv"}, Body: "draft reply", Pending: true},
				},
			},
		},
	}
}

func withPending(t *testing.T) Model {
	t.Helper()
	m := loaded(t, 120, 40)
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: pendingFixture()})
	return m
}

func TestAnOpenThreadIsShownUnderItsLine(t *testing.T) {
	out := ansi.Strip(withThreads(t, 120, 40).View())
	if !strings.Contains(out, "is 2 not the default?") {
		t.Errorf("the open thread is not on screen:\n%s", out)
	}
	if !strings.Contains(out, "kukv") {
		t.Errorf("the thread does not name its author:\n%s", out)
	}
}

// openCollapsedThread moves the cursor onto the collapsed row, ready for
// enter to open it.
func openCollapsedThread(m Model) Model {
	for i, r := range m.rows {
		if r.kind == rowCollapsed {
			m.row = i
			break
		}
	}
	return m
}

func TestASettledThreadIsACountUntilItIsOpened(t *testing.T) {
	m := withThreads(t, 120, 40)
	out := ansi.Strip(m.View())
	if strings.Contains(out, "settled long ago") {
		t.Errorf("a resolved thread is shown in full before it is opened:\n%s", out)
	}
	if !strings.Contains(out, "settled comment") {
		t.Errorf("the resolved thread is not counted:\n%s", out)
	}

	m = press(openCollapsedThread(m), "enter")
	if !strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Errorf("enter did not open the settled thread:\n%s", ansi.Strip(m.View()))
	}
	m = press(m, "enter")
	if strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Errorf("enter did not close the settled thread again")
	}
}

// settledThreadWithTwoComments is one resolved thread with two comments, so
// opening it draws two rowThread rows sharing the same key.
func settledThreadWithTwoComments() gh.ReviewContext {
	return gh.ReviewContext{
		PullRequestID: "PR_1",
		Threads: []gh.ReviewThread{
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideRight, Resolved: true,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "kukv"}, Body: "first"},
					{Author: gh.Author{Login: "someone"}, Body: "second"},
				},
			},
		},
	}
}

// TestEnterClosesAnOpenThreadFromAnyOfItsRows guards the reason toggleCollapsed
// accepts rowThread as well as rowCollapsed: an open, multi-comment thread
// draws one row per comment, and enter has to close the whole group from
// whichever of those rows the cursor is on, not only the first.
func TestEnterClosesAnOpenThreadFromAnyOfItsRows(t *testing.T) {
	m := loaded(t, 120, 40)
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: settledThreadWithTwoComments()})
	m = press(openCollapsedThread(m), "enter")
	if !strings.Contains(ansi.Strip(m.View()), "second") {
		t.Fatalf("enter did not open the thread:\n%s", ansi.Strip(m.View()))
	}

	// Move the cursor onto the second comment's row before closing it.
	last := -1
	for i, r := range m.rows {
		if r.kind == rowThread {
			last = i
		}
	}
	if last < 0 {
		t.Fatal("no rowThread rows after opening the thread")
	}
	m.row = last

	m = press(m, "enter")
	if strings.Contains(ansi.Strip(m.View()), "second") {
		t.Errorf("enter on the last comment row did not close the thread:\n%s", ansi.Strip(m.View()))
	}
}

// TestACommentOnALineThisDiffDoesNotShowIsStillVisible: dropping it would
// hide the fact that someone objected at all.
func TestACommentOnALineThisDiffDoesNotShowIsStillVisible(t *testing.T) {
	out := ansi.Strip(withThreads(t, 120, 40).View())
	if !strings.Contains(out, "on a line not in this diff") {
		t.Errorf("an unplaceable comment was dropped:\n%s", out)
	}
}

// TestAnUnsentCommentIsMarked checks the marker that tells the user what
// pressing x on that comment would throw away, and the header's count of how
// many are still waiting to go out.
func TestAnUnsentCommentIsMarked(t *testing.T) {
	out := ansi.Strip(withPending(t).View())
	if !strings.Contains(out, "not sent yet") {
		t.Errorf("the pending comment is not marked unsent:\n%s", out)
	}
	if !strings.Contains(out, "pending · 1") {
		t.Errorf("the header does not count the pending comment:\n%s", out)
	}
}

// TestSidebarShowsAThreadCountBadge covers spec 4.4.1's "●2": the sidebar
// draws a file's thread count next to its size, and says nothing at all for
// a file with no threads.
func TestSidebarShowsAThreadCountBadge(t *testing.T) {
	m := withThreads(t, 120, 40)
	lines := m.sidebarLines()

	out := strings.Join(lines, "\n")
	if !strings.Contains(ansi.Strip(out), "●3") {
		t.Errorf("graph/walk.go has 3 threads but no badge is drawn:\n%s", ansi.Strip(out))
	}
	if strings.Contains(ansi.Strip(out), "●0") {
		t.Errorf("a zero count must draw nothing, not ●0:\n%s", ansi.Strip(out))
	}

	// logo.png is the second file in the fixture (lines 2 and 3: path, then
	// size) and has no threads at all, so its size line must carry no badge.
	if strings.Contains(ansi.Strip(lines[3]), "●") {
		t.Errorf("logo.png has no threads but drew a badge: %q", ansi.Strip(lines[3]))
	}

	for _, l := range lines {
		if w := ansi.StringWidth(l); w > sidebarWidth {
			t.Errorf("sidebar line %q is %d columns wide, want at most %d", ansi.Strip(l), w, sidebarWidth)
		}
	}
}

// TestAnswersForAnotherPullRequestAreDroppedForReview mirrors the diff
// fetch's own guard: a review context that lands for the pull request the
// user just left must not overwrite this one's.
func TestAnswersForAnotherPullRequestAreDroppedForReview(t *testing.T) {
	m := loaded(t, 120, 30)
	other := gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 999}
	m, _ = m.Update(reviewMsg{ref: other, ctx: threadFixture()})
	if len(m.review.Threads) != 0 {
		t.Errorf("a review context for %v landed on this view's model", other)
	}
}

// multiLineReview builds a review context with one comment whose body is the
// given multi-line text.
func multiLineReview(body string) gh.ReviewContext {
	return gh.ReviewContext{
		PullRequestID: "PR_1",
		Threads: []gh.ReviewThread{
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideRight,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "kukv"}, Body: body},
				},
			},
		},
	}
}

// TestAMultiLineCommentStaysOnOneRow is the newline sibling of the tab bug
// this package already fixed once: a review comment is markdown and
// routinely has blank lines in it, and View joins one string per row, so a
// body carrying its own newline would draw as extra visual rows and shift
// every row under it down by one.
func TestAMultiLineCommentStaysOnOneRow(t *testing.T) {
	single := loaded(t, 120, 40)
	single, _ = single.Update(reviewMsg{ref: single.ref, ctx: multiLineReview("this needs a guard see the issue for why")})
	want := len(strings.Split(single.View(), "\n"))

	multi := loaded(t, 120, 40)
	multi, _ = multi.Update(reviewMsg{ref: multi.ref, ctx: multiLineReview("this needs a guard\n\nsee the issue for why")})
	got := len(strings.Split(multi.View(), "\n"))

	if got != want {
		t.Errorf("a multi-line comment drew %d rows, want %d (one row, same as a single-line body)", got, want)
	}
	if !strings.Contains(ansi.Strip(multi.View()), "this needs a guard") {
		t.Errorf("the comment text is missing:\n%s", ansi.Strip(multi.View()))
	}
}

// TestAMultiLineJapaneseCommentStaysOnOneRow guards the width arithmetic:
// folding whitespace must not disturb the double-width counting this package
// has already fought for twice.
func TestAMultiLineJapaneseCommentStaysOnOneRow(t *testing.T) {
	m := loaded(t, 120, 40)
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: multiLineReview("ここは直したほうがいい\n\n理由は issue を見て")})
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "ここは直したほうがいい") {
		t.Errorf("the Japanese comment text is missing:\n%s", out)
	}
	for i, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 120 {
			t.Errorf("line %d is %d columns wide in a 120-wide terminal: %q", i, w, ansi.Strip(line))
		}
	}
}

// TestSidesAreNotMixedUp is the test that stops a comment on the old version
// of a line being drawn under the new one.
func TestSidesAreNotMixedUp(t *testing.T) {
	m := withThreads(t, 120, 40)
	left := m.threadsFor("graph/walk.go", 13, gh.SideLeft)
	right := m.threadsFor("graph/walk.go", 13, gh.SideRight)
	if len(left) != 1 || !left[0].Resolved {
		t.Errorf("the left side of line 13 has %+v, want the resolved thread", left)
	}
	if len(right) != 1 || right[0].Resolved {
		t.Errorf("the right side of line 13 has %+v, want the open thread", right)
	}
}
