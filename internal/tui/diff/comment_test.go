package diff

import (
	"errors"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// recordingSource remembers what was sent, which is the only thing worth
// asserting: nothing here talks to GitHub.
type recordingSource struct {
	fakeSource
	mu           sync.Mutex
	started      int
	comments     []gh.PendingComment
	reviewID     string
	discardCalls int
}

func (s *recordingSource) StartReview(string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started++
	s.reviewID = "PRR_new"
	return s.reviewID, nil
}

func (s *recordingSource) AddReviewThread(_ string, c gh.PendingComment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.comments = append(s.comments, c)
	return nil
}

func (s *recordingSource) DiscardReview(string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.discardCalls++
	return nil
}

// loadedWith is loaded with a review context handed over as well as the
// diff: without a pull request id, c must do nothing (see
// TestCDoesNothingBeforeTheContextArrives), so every test that wants c to
// work needs this rather than loaded. The size is the same 120x40 withThreads
// and withPending use: wide enough that a composed line does not wrap, tall
// enough that every fixture row is on screen at once.
func loadedWith(t *testing.T, src Source) Model {
	t.Helper()
	return loadedWithAt(t, src, 120, 40)
}

func loadedWithAt(t *testing.T, src Source, width, height int) Model {
	t.Helper()
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(diffMsg{ref: m.ref, files: fixture()})
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: gh.ReviewContext{PullRequestID: "PR_1"}})
	return m
}

// cursorOnLine puts the cursor on the rowLine that quotes the given kind and
// line number, using DiffLine.Line() the same way a real comment would, so a
// test that asks for line 13 on the right gets the added line and one that
// asks for the left gets the removed line. It fails the test outright rather
// than leaving the cursor where it was, so a typo in the fixture cannot pass
// silently.
func cursorOnLine(t *testing.T, m Model, kind gh.DiffLineKind, num int) Model {
	t.Helper()
	for i, r := range m.rows {
		if r.kind != rowLine || r.line.Kind != kind {
			continue
		}
		if line, _ := r.line.Line(); line == num {
			m.row = i
			return m
		}
	}
	t.Fatalf("no rowLine with kind %v quoting line %d", kind, num)
	return m
}

// typeInto sends one KeyPressMsg per rune, the way a user typing into the
// composer would.
func typeInto(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// keyPress is key's name in the brief; both build the same KeyPressMsg.
func keyPress(s string) tea.KeyMsg { return key(s) }

// runCmd runs a command purely for its side effects (what it sends to
// src), without feeding the message it returns back into the model.
func runCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run")
	}
	cmd()
}

// runInto runs a command and feeds the single message it returns into the
// model, without chasing any further command that answer produces (the way
// the real program's runtime would). That is enough to observe composing,
// posting and the reused review id settle after one send.
func runInto(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run")
	}
	m, _ = m.Update(cmd())
	return m
}

func TestCommentingOnAnAddedLineQuotesTheRightSide(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)

	// Put the cursor on the added line "if depth <= 0 {".
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	if !m.composing {
		t.Fatal("c did not open the composer")
	}
	m = typeInto(m, "why not 2?")
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if len(src.comments) != 1 {
		t.Fatalf("%d comments sent, want 1", len(src.comments))
	}
	got := src.comments[0]
	want := gh.PendingComment{Path: "graph/walk.go", Line: 13, Side: gh.SideRight, Body: "why not 2?"}
	if got != want {
		t.Errorf("sent %+v, want %+v", got, want)
	}
}

func TestCommentingOnARemovedLineQuotesTheLeftSide(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	m = cursorOnLine(t, m, gh.LineRemoved, 13)
	m = press(m, "c")
	m = typeInto(m, "why?")
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if len(src.comments) != 1 {
		t.Fatalf("%d comments sent, want 1", len(src.comments))
	}
	if got := src.comments[0].Side; got != gh.SideLeft {
		t.Errorf("side = %v, want the left: the line was removed", got)
	}
}

// TestTheSecondCommentReusesTheReview: starting a review per comment would
// leave a pile of separate reviews on the pull request.
func TestTheSecondCommentReusesTheReview(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	for _, body := range []string{"first", "second"} {
		m = cursorOnLine(t, m, gh.LineAdded, 13)
		m = press(m, "c")
		m = typeInto(m, body)
		var cmd tea.Cmd
		m, cmd = m.Update(keyPress("ctrl+s"))
		m = runInto(t, m, cmd)
	}
	if src.started != 1 {
		t.Errorf("started %d reviews, want 1", src.started)
	}
	if len(src.comments) != 2 {
		t.Errorf("%d comments sent, want 2", len(src.comments))
	}
}

// TestASecondCommentBeforeTheRefetchLandsStillReusesTheReview: post() reads
// m.review.PendingID, but the fetch that would confirm that id from GitHub
// is asynchronous -- commentPostedMsg sets PendingID itself, synchronously,
// precisely so a second c sent before that fetch's answer arrives still
// finds it set. Getting this wrong calls StartReview twice and leaves two
// pending reviews open on the pull request.
func TestASecondCommentBeforeTheRefetchLandsStillReusesTheReview(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)

	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	m = typeInto(m, "first")
	m, cmd := m.Update(keyPress("ctrl+s"))
	posted := cmd() // runs the network side: StartReview + AddReviewThread
	m, _ = m.Update(posted)
	// posted's own fetchReview command is deliberately never run: the
	// refetch's answer must not be what makes PendingID available.

	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	m = typeInto(m, "second")
	_, cmd = m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if src.started != 1 {
		t.Errorf("started %d reviews, want 1", src.started)
	}
	if len(src.comments) != 2 {
		t.Errorf("%d comments sent, want 2", len(src.comments))
	}
}

// TestCWaitsForThePullRequestID: the diff and the review context are fetched
// in parallel. If the diff lands first and c opened the composer, sending
// would call StartReview with an empty node id.
func TestCDoesNothingBeforeTheContextArrives(t *testing.T) {
	m := loaded(t, 120, 40) // the diff only
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	if m.composing {
		t.Error("c opened the composer before the pull request's id was known")
	}
}

// TestCDoesNothingOnAHunkHeader has to be built with loadedWith, not loaded:
// with loaded, m.review.PullRequestID is always "" (no reviewMsg ever
// arrives), so the r.kind != rowLine clause is not what makes the test pass
// -- the PullRequestID guard alone already suppresses composing regardless of
// the cursor. Deleting r.kind != rowLine must make this fail; it did not
// against the old loaded-based version.
func TestCDoesNothingOnAHunkHeader(t *testing.T) {
	m := loadedWith(t, &recordingSource{fakeSource: fakeSource{files: fixture()}})
	m.row = 0 // the first row of the fixture is a hunk header
	m = press(m, "c")
	if m.composing {
		t.Error("c opened the composer on a hunk header")
	}
}

// TestCDoesNothingOnAThreadRow is rowLine's other neighbour: a thread has no
// line of its own to comment on either.
func TestCDoesNothingOnAThreadRow(t *testing.T) {
	m := loadedWith(t, &recordingSource{fakeSource: fakeSource{files: fixture()}})
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: threadFixture()})
	found := false
	for i, r := range m.rows {
		if r.kind == rowThread {
			m.row = i
			found = true
			break
		}
	}
	if !found {
		t.Fatal("no rowThread in the fixture; this test proves nothing")
	}
	m = press(m, "c")
	if m.composing {
		t.Error("c opened the composer on a thread row")
	}
}

// TestCOnAHunkHeaderSaysWhyNothingHappened is the c-silent fix's other half:
// the guard in startComposing is correct (a hunk header has no line), but it
// used to leave the screen unchanged, indistinguishable from the key doing
// nothing at all.
func TestCOnAHunkHeaderSaysWhyNothingHappened(t *testing.T) {
	m := loadedWith(t, &recordingSource{fakeSource: fakeSource{files: fixture()}})
	m.row = 0 // the first row of the fixture is a hunk header
	m = press(m, "c")
	if m.composing {
		t.Fatal("c opened the composer on a hunk header")
	}
	if m.declined != i18n.T("diff.decline_no_line") {
		t.Errorf("declined = %q, want the no-line message", m.declined)
	}
	if !strings.Contains(ansi.Strip(m.View()), i18n.T("diff.decline_no_line")) {
		t.Errorf("the no-line message is not on screen:\n%s", ansi.Strip(m.View()))
	}
}

// TestCBeforeTheContextArrivesSaysItIsLoading is TestCDoesNothingBeforeThe-
// ContextArrives with the message on top: for the ~6.5s the review context
// takes to fetch, c is inert, and the viewer needs to know it is loading
// rather than broken.
func TestCBeforeTheContextArrivesSaysItIsLoading(t *testing.T) {
	m := loaded(t, 120, 40) // the diff only
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	if m.composing {
		t.Fatal("c opened the composer before the pull request's id was known")
	}
	if m.declined != i18n.T("diff.decline_loading") {
		t.Errorf("declined = %q, want the loading message", m.declined)
	}
}

// TestDecliningKeysAddNoSecondMessageWhenTheReviewContextNeverArrived: c, v
// and X all guard "the review context has never arrived" with an "unless
// reviewErr already says so" clause, so a single failed fetch (before any
// reviewMsg) does not draw the loading message on top of reviewErr's own
// footer line. This is built with loaded, not loadedWith: with loaded,
// m.review.PullRequestID is always "" (no reviewMsg ever arrives), which is
// exactly the case the guard exists for. Each guard is deleted in turn while
// developing this test to confirm the corresponding row fails without it.
func TestDecliningKeysAddNoSecondMessageWhenTheReviewContextNeverArrived(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		needsLine bool // c also declines on a non-line row, so it needs the cursor moved first
		acted     func(m Model) bool
	}{
		{name: "c", key: "c", needsLine: true, acted: func(m Model) bool { return m.composing }},
		{name: "v", key: "v", acted: func(m Model) bool { return m.submitting }},
		{name: "X", key: "X", acted: func(m Model) bool { return m.discarding }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, 120, 30)
			if tt.needsLine {
				m = cursorOnLine(t, m, gh.LineAdded, 13)
			}
			m, _ = m.Update(reviewErrMsg{ref: m.ref, err: errors.New("boom from github")})
			m = press(m, tt.key)
			if tt.acted(m) {
				t.Fatalf("%s acted even though the review context has never arrived", tt.key)
			}
			if m.declined != "" {
				t.Errorf("declined = %q, want empty: reviewErr already says this", m.declined)
			}
		})
	}
}

// TestCStillWorksAfterASecondReviewContextFetchFails is the regression this
// wave exists for: reviewErrMsg only means the last refetch failed, it never
// touches m.review (diff.go's reviewErrMsg case sets m.reviewErr alone), so a
// pull request id and a pending review confirmed by an earlier, successful
// fetch are still good. c used to check reviewErr before PullRequestID and
// decline unconditionally, which meant posting a comment, letting the
// automatic refetch after it hiccup, and losing c for the rest of the
// session -- exactly the silence this branch was written to remove.
func TestCStillWorksAfterASecondReviewContextFetchFails(t *testing.T) {
	m := withThreads(t, 120, 40) // a real PullRequestID from threadFixture
	m, _ = m.Update(reviewErrMsg{ref: m.ref, err: errors.New("boom from github")})
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	if !m.composing {
		t.Fatal("c is dead after a review-context refetch failure, though the pull request id is known")
	}
	if m.declined != "" {
		t.Errorf("declined = %q, want empty: reviewErr already says this", m.declined)
	}
}

// TestTheDeclineMessageClearsWhenTheCursorMoves: the message stops being
// true the moment the viewer moves off the row it was about.
func TestTheDeclineMessageClearsWhenTheCursorMoves(t *testing.T) {
	m := loadedWith(t, &recordingSource{fakeSource: fakeSource{files: fixture()}})
	m.row = 0
	m = press(m, "c")
	if m.declined == "" {
		t.Fatal("no decline message to begin with; this test proves nothing")
	}
	m = press(m, "j")
	if m.declined != "" {
		t.Errorf("declined = %q after moving the cursor, want cleared", m.declined)
	}
}

func TestEscThrowsTheDraftAway(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	// Any added line does for this test; line 14 rather than 13 shows the
	// composer does not care which one the cursor was on.
	m = cursorOnLine(t, m, gh.LineAdded, 14)
	m = press(m, "c")
	m = typeInto(m, "never mind")
	m, _ = m.Update(keyPress("esc"))
	if m.composing {
		t.Error("esc did not close the composer")
	}
	if len(src.comments) != 0 {
		t.Errorf("esc sent %d comments", len(src.comments))
	}
	if strings.Contains(ansi.Strip(m.View()), "never mind") {
		t.Error("the discarded draft is still on screen")
	}
}

// TestTheComposerFitsTheTerminal is TestTheDiffFitsTheTerminal's counterpart
// for the composer: its rows come out of the pane's height budget, so the
// key bar must stay on screen the same way a review failure's line does.
func TestTheComposerFitsTheTerminal(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	for _, width := range []int{80, 120} {
		for _, height := range []int{24, 40} {
			m := loadedWithAt(t, src, width, height)
			m = cursorOnLine(t, m, gh.LineAdded, 13)
			m = press(m, "c")
			out := m.View()
			if got := len(strings.Split(out, "\n")); got > height {
				t.Errorf("the composer drew %d lines into a %dx%d terminal", got, width, height)
			}
			if !strings.Contains(ansi.Strip(out), ansi.Strip(m.keyBar())) {
				t.Errorf("the key bar was pushed off the screen:\n%s", ansi.Strip(out))
			}
		}
	}
}

// TestTheComposerShowsItsOwnPlaceholderAndFooter checks that the composer
// draws with diff-specific strings, not detail's compose.* catalog entries.
func TestTheComposerShowsItsOwnPlaceholderAndFooter(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	out := ansi.Strip(m.View())
	if !strings.Contains(out, i18n.T("diff.comment_placeholder")) {
		t.Errorf("the composer does not show its placeholder:\n%s", out)
	}
	if !strings.Contains(out, i18n.T("footer.diff_comment")) {
		t.Errorf("the key bar was not swapped for the composer's:\n%s", out)
	}
}

// TestAFailedPostKeepsTheDraftAndShowsTheError is comment.go's share of
// .claude/rules/errors.md's Bubble Tea section: a failed post is a
// footer-level failure, and what the user typed must not vanish.
func TestAFailedPostKeepsTheDraftAndShowsTheError(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	m = typeInto(m, "why not 2?")
	m, _ = m.Update(keyPress("ctrl+s")) // the post cmd is deliberately not run

	m, _ = m.Update(commentErrorMsg{ref: m.ref, err: errors.New("422: line not part of the diff")})
	if !m.composing {
		t.Error("composing = false after a failed post, want still composing")
	}
	if m.posting {
		t.Error("posting = true after a failed post, want false")
	}
	if got := m.textarea.Value(); got != "why not 2?" {
		t.Errorf("draft lost: textarea = %q, want %q", got, "why not 2?")
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "why not 2?") {
		t.Errorf("the draft is not shown after the failed post:\n%s", out)
	}
	if !strings.Contains(out, "422: line not part of the diff") {
		t.Errorf("the failure is not shown:\n%s", out)
	}
}

// TestAPostedCommentReusesTheReviewIDAndRefetches checks the two things
// commentPostedMsg is responsible for: the id a second comment would reuse,
// and asking for the review context again so the viewer's own comment comes
// back as a Pending thread.
func TestAPostedCommentReusesTheReviewIDAndRefetches(t *testing.T) {
	m := loaded(t, 120, 40)
	m, cmd := m.Update(commentPostedMsg{ref: m.ref, reviewID: "PRR_1"})
	if m.composing || m.posting {
		t.Errorf("composing = %v, posting = %v after commentPostedMsg, want both false", m.composing, m.posting)
	}
	if m.review.PendingID != "PRR_1" {
		t.Errorf("review.PendingID = %q, want %q", m.review.PendingID, "PRR_1")
	}
	if cmd == nil {
		t.Fatal("commentPostedMsg produced no command to refetch the review context")
	}
	if _, ok := cmd().(reviewMsg); !ok && !isReviewErrMsg(cmd()) {
		t.Errorf("commentPostedMsg's command produced %T, want a review fetch", cmd())
	}
}

func isReviewErrMsg(msg tea.Msg) bool {
	_, ok := msg.(reviewErrMsg)
	return ok
}

// TestARefetchMidComposeDoesNotShiftThePostedTarget is the finding this fix
// pass is for. The cursor is put on the added line "depth = defaultDepth"
// (14), c captures that as the target, and only then does a reviewMsg land
// that inserts one thread row directly under line 13 -- above the cursor,
// pushing it down by exactly one row. Without a captured target, post() would
// read whatever row the cursor's raw index now names, which is the inserted
// thread row: its zero gh.DiffLine reads as line 0, and the comment goes
// there instead of line 14.
func TestARefetchMidComposeDoesNotShiftThePostedTarget(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	m = cursorOnLine(t, m, gh.LineAdded, 14)
	m = press(m, "c")
	if !m.composing {
		t.Fatal("c did not open the composer")
	}
	m = typeInto(m, "still about line 14")

	shifted := gh.ReviewContext{
		PullRequestID: "PR_1",
		Threads: []gh.ReviewThread{
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideRight,
				Comments: []gh.ThreadComment{{Author: gh.Author{Login: "kukv"}, Body: "shifts the row below"}},
			},
		},
	}
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: shifted})

	if got := m.currentRow().kind; got != rowThread {
		t.Fatalf("the fixture did not shift the cursor onto a thread row (got %v); this test proves nothing", got)
	}

	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if len(src.comments) != 1 {
		t.Fatalf("%d comments sent, want 1", len(src.comments))
	}
	want := gh.PendingComment{Path: "graph/walk.go", Line: 14, Side: gh.SideRight, Body: "still about line 14"}
	if got := src.comments[0]; got != want {
		t.Errorf("sent %+v, want %+v", got, want)
	}
}

// TestARetryAfterAFailedPostStillTargetsTheCapturedLine checks that the
// target survives the error path too. commentErrorMsg reopens the composer
// (m.composing = true) without going through startComposing again, so if
// post() cleared m.target on every send rather than only on success, a retry
// here would post to the zero value instead of line 13.
func TestARetryAfterAFailedPostStillTargetsTheCapturedLine(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	m = typeInto(m, "retry me")
	m, _ = m.Update(keyPress("ctrl+s")) // the post cmd is deliberately not run

	m, _ = m.Update(commentErrorMsg{ref: m.ref, err: errors.New("500")})
	if !m.composing {
		t.Fatal("composing = false after a failed post, want still composing")
	}

	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if len(src.comments) != 1 {
		t.Fatalf("%d comments sent, want 1", len(src.comments))
	}
	want := gh.PendingComment{Path: "graph/walk.go", Line: 13, Side: gh.SideRight, Body: "retry me"}
	if got := src.comments[0]; got != want {
		t.Errorf("sent %+v, want %+v", got, want)
	}
}

// TestARefetchReclampsTheCursorIntoRange: a reviewMsg can shrink m.rows (the
// viewer's own thread stays until a refetch lands, threads can resolve, and
// so on). If m.row is left pointing past the end, currentRow and diffLines
// index out of range.
func TestARefetchReclampsTheCursorIntoRange(t *testing.T) {
	m := loadedWith(t, &recordingSource{fakeSource: fakeSource{files: fixture()}})
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: threadFixture()})
	m.row = len(m.rows) - 1 // parked at the end of the widened row set

	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: gh.ReviewContext{PullRequestID: "PR_1"}}) // threads gone
	if m.row >= len(m.rows) {
		t.Fatalf("row = %d out of range for %d rows", m.row, len(m.rows))
	}
	_ = m.View() // must not panic
}

// TestACommentErrorForAnotherPullRequestIsDropped mirrors
// TestAReviewFailureForAnotherPullRequestIsDropped: a post for the pull
// request the user has since left must not surface here.
// TestVOpensThePopupWithOrWithoutAPendingReview: approving a diff you had
// nothing to say about is the commonest review there is.
func TestVOpensThePopupWithOrWithoutAPendingReview(t *testing.T) {
	m := withThreads(t, 120, 40) // the context has arrived; nothing waiting
	m.review.PendingID = ""
	m = press(m, "v")
	if !m.submitting {
		t.Error("v did nothing with no pending review; approving needs no comments")
	}

	m2 := withThreads(t, 120, 40)
	m2.review.PendingID = "PRR_9"
	m2 = press(m2, "v")
	if !m2.submitting {
		t.Error("v did not open the popup when a review was waiting")
	}
}

// TestVDoesNothingBeforeTheContextArrives: the diff and the review context
// are fetched in parallel, so v can be pressed while only the diff has
// landed. Opening then would submit against an empty node id.
func TestVDoesNothingBeforeTheContextArrives(t *testing.T) {
	m := loaded(t, 120, 40) // the diff only
	m = press(m, "v")
	if m.submitting {
		t.Error("v opened the popup before the pull request's id was known")
	}
}

// TestVBeforeTheContextArrivesSaysItIsLoading is v's share of the same fix
// c got: it has the same PullRequestID guard, and the same silence.
func TestVBeforeTheContextArrivesSaysItIsLoading(t *testing.T) {
	m := loaded(t, 120, 40) // the diff only
	m = press(m, "v")
	if m.submitting {
		t.Fatal("v opened the popup before the pull request's id was known")
	}
	if m.declined != i18n.T("diff.decline_loading") {
		t.Errorf("declined = %q, want the loading message", m.declined)
	}
}

// TestXBeforeTheContextArrivesSaysItIsLoading is X's share of the same fix c
// and v got: PendingID == "" before the context has arrived is not "no
// pending review" but "not known yet", and the two need different messages
// -- one says wait, the other says there is nothing to do.
func TestXBeforeTheContextArrivesSaysItIsLoading(t *testing.T) {
	m := loaded(t, 120, 40) // the diff only
	m = press(m, "X")
	if m.discarding {
		t.Fatal("X asked to discard before the pull request's id was known")
	}
	if m.declined != i18n.T("diff.decline_loading") {
		t.Errorf("declined = %q, want the loading message", m.declined)
	}
}

func TestXWithNoPendingReviewSaysWhyNothingHappened(t *testing.T) {
	m := withThreads(t, 120, 40)
	m.review.PendingID = ""
	m = press(m, "X")
	if m.discarding {
		t.Fatal("X asked to discard with no pending review")
	}
	if m.declined != i18n.T("diff.decline_no_pending_review") {
		t.Errorf("declined = %q, want the no-pending-review message", m.declined)
	}
}

func TestCapitalXAsksBeforeDiscarding(t *testing.T) {
	m := withThreads(t, 120, 40)
	m.review.PendingID = "PRR_9"
	m = press(m, "X")
	if !m.discarding {
		t.Fatal("X did not ask")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Discard") {
		t.Errorf("the question is not on screen:\n%s", ansi.Strip(m.View()))
	}
	m = press(m, "n")
	if m.discarding {
		t.Error("n did not take the question away")
	}
}

// TestASecondYWhileDiscardingDoesNotCallDiscardTwice guards discardWorking:
// once the first y has sent DiscardReview and is waiting on its answer, a
// second y arriving before that answer lands must not fire a second call.
func TestASecondYWhileDiscardingDoesNotCallDiscardTwice(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: threadFixture()})
	m.review.PendingID = "PRR_9"
	m = press(m, "X")

	m, cmd1 := m.Update(keyPress("y")) // starts the discard; DiscardReview not yet run
	m, cmd2 := m.Update(keyPress("y")) // must be ignored: the first discard is still in flight
	runCmd(t, cmd1)
	if cmd2 != nil {
		cmd2()
	}

	if src.discardCalls != 1 {
		t.Errorf("%d DiscardReview calls, want 1", src.discardCalls)
	}
}

// TestTheSubmitPopupFitsTheTerminal is TestTheComposerFitsTheTerminal's
// counterpart for the review popup: its rows come out of the pane's height
// budget, so the key bar must stay on screen the same way the composer's
// does.
func TestTheSubmitPopupFitsTheTerminal(t *testing.T) {
	for _, width := range []int{80, 120} {
		for _, height := range []int{24, 40} {
			m := withThreads(t, width, height)
			m.review.PendingID = "PRR_9"
			m = press(m, "v")
			out := m.View()
			if got := len(strings.Split(out, "\n")); got > height {
				t.Errorf("the submit popup drew %d lines into a %dx%d terminal", got, width, height)
			}
			if !strings.Contains(ansi.Strip(out), ansi.Strip(m.keyBar())) {
				t.Errorf("the key bar was pushed off the screen:\n%s", ansi.Strip(out))
			}
		}
	}
}

// TestTheDiscardConfirmationFitsTheTerminal is the discard confirmation's
// share of the same budget.
func TestTheDiscardConfirmationFitsTheTerminal(t *testing.T) {
	for _, width := range []int{80, 120} {
		for _, height := range []int{24, 40} {
			m := withThreads(t, width, height)
			m.review.PendingID = "PRR_9"
			m = press(m, "X")
			out := m.View()
			if got := len(strings.Split(out, "\n")); got > height {
				t.Errorf("the discard confirmation drew %d lines into a %dx%d terminal", got, width, height)
			}
			if !strings.Contains(ansi.Strip(out), ansi.Strip(m.keyBar())) {
				t.Errorf("the key bar was pushed off the screen:\n%s", ansi.Strip(out))
			}
		}
	}
}

func TestACommentErrorForAnotherPullRequestIsDropped(t *testing.T) {
	src := &recordingSource{fakeSource: fakeSource{files: fixture()}}
	m := loadedWith(t, src)
	m = cursorOnLine(t, m, gh.LineAdded, 13)
	m = press(m, "c")
	m = typeInto(m, "why not 2?")
	m, _ = m.Update(keyPress("ctrl+s")) // the post cmd is deliberately not run

	other := gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 999}
	m, _ = m.Update(commentErrorMsg{ref: other, err: errors.New("boom")})
	if !m.posting {
		t.Error("posting = false after an error for another pull request, want still posting")
	}
	if m.postErr != "" {
		t.Errorf("postErr = %q after an error for another pull request, want empty", m.postErr)
	}
}
