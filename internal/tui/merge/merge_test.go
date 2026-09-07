package merge

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	ctx gh.MergeContext
	// seq is one answer per fetch, in the order the fetches are run. It is
	// what tells a stale answer from a fresh one; without it every fetch
	// answers the same and the two are indistinguishable.
	seq     []gh.MergeContext
	fetches int
	err     error
	merged  []gh.MergeMethod
	enabled []gh.MergeMethod
	offCall int
}

func (f *fakeSource) PRMergeContext(context.Context, string, int) (gh.MergeContext, error) {
	i := f.fetches
	f.fetches++
	if i < len(f.seq) {
		return f.seq[i], f.err
	}
	return f.ctx, f.err
}

func (f *fakeSource) MergePR(_ string, m gh.MergeMethod) error {
	f.merged = append(f.merged, m)
	return nil
}

func (f *fakeSource) EnableAutoMerge(_ string, m gh.MergeMethod) error {
	f.enabled = append(f.enabled, m)
	return nil
}

func (f *fakeSource) DisableAutoMerge(string) error {
	f.offCall++
	return nil
}

func ref() gh.ItemRef {
	return gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61}
}

// loaded runs New's fetch and hands the answer back, the way Bubble Tea
// would: the test never builds the loaded Model by hand.
func loaded(t *testing.T, f *fakeSource) Model {
	t.Helper()

	m := New(f, ref())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned no command: nothing fetches the merge context")
	}
	m, _ = m.Update(cmd())
	return m
}

func press(m Model, key string) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}

func enter(m Model) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func escape(m Model) (Model, tea.Cmd) {
	return m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
}

func autoMergeable() gh.MergeContext {
	c := mergeable()
	c.AutoMergeAllowed = true
	c.ViewerCanEnableAutoMerge = true
	return c
}

func mergeable() gh.MergeContext {
	return gh.MergeContext{
		PullRequestID: "PR_1",
		Mergeable:     gh.MergeableYes,
		State:         gh.MergeStateUnstable,
		Methods:       []gh.MergeMethod{gh.MergeSquash, gh.MergeCommit, gh.MergeRebase},
	}
}

func TestEnterMergesWithTheMethodOnTheCursor(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: mergeable()}
	m := loaded(t, f)
	m, _ = press(m, "j")
	_, cmd := enter(m)
	if cmd == nil {
		t.Fatal("enter returned no command: nothing was sent")
	}
	if msg := cmd(); msg != (MergedMsg{Merged: true}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{Merged: true}", msg)
	}
	if len(f.merged) != 1 || f.merged[0] != gh.MergeCommit {
		t.Errorf("merged = %v, want one MergeCommit (j moved off squash)", f.merged)
	}
}

func TestTheCursorStaysInsideWhatTheRepositoryAllows(t *testing.T) {
	t.Parallel()

	only := mergeable()
	only.Methods = []gh.MergeMethod{gh.MergeRebase}
	f := &fakeSource{ctx: only}
	m := loaded(t, f)
	m, _ = press(m, "j")
	m, _ = press(m, "j")
	_, cmd := enter(m)
	_ = cmd()
	if len(f.merged) != 1 || f.merged[0] != gh.MergeRebase {
		t.Errorf("merged = %v, want one MergeRebase: j must not walk past the only method", f.merged)
	}
}

func TestEnterIsRefusedWhileGitHubIsStillWorkingItOut(t *testing.T) {
	t.Parallel()

	computing := mergeable()
	computing.Mergeable = gh.MergeableUnknown
	computing.State = gh.MergeStateUnknown
	f := &fakeSource{ctx: computing}
	m := loaded(t, f)
	_, cmd := enter(m)
	if cmd != nil {
		t.Fatal("enter sent something while mergeable was UNKNOWN")
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing", f.merged)
	}
}

func TestSpaceQueuesTheMergeBehindTheChecks(t *testing.T) {
	t.Parallel()

	auto := mergeable()
	auto.AutoMergeAllowed = true
	auto.ViewerCanEnableAutoMerge = true
	f := &fakeSource{ctx: auto}
	m := loaded(t, f)
	m, _ = press(m, " ")
	_, cmd := enter(m)
	if msg := cmd(); msg != (MergedMsg{}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{}: the pull request is only queued, not merged", msg)
	}
	if len(f.enabled) != 1 {
		t.Fatalf("enabled = %v, want one call: space chose auto-merge", f.enabled)
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing: auto-merge does not merge now", f.merged)
	}
}

func TestAutoMergeCannotBeChosenOnAPullRequestWithNothingToWaitFor(t *testing.T) {
	t.Parallel()

	clean := mergeable()
	clean.State = gh.MergeStateClean
	clean.AutoMergeAllowed = true
	clean.ViewerCanEnableAutoMerge = true
	f := &fakeSource{ctx: clean}
	m := loaded(t, f)
	m, _ = press(m, " ")
	_, cmd := enter(m)
	_ = cmd()
	if len(f.enabled) != 0 {
		t.Errorf("enabled = %v, want nothing: GitHub refuses auto-merge on a clean pull request", f.enabled)
	}
	if len(f.merged) != 1 {
		t.Errorf("merged = %v, want one: enter still merges", f.merged)
	}
}

func TestEnterCancelsAnAutoMergeThatIsAlreadyOn(t *testing.T) {
	t.Parallel()

	on := mergeable()
	on.AutoMergeAllowed = true
	on.ViewerCanEnableAutoMerge = true
	on.AutoMergeEnabled = true
	f := &fakeSource{ctx: on}
	m := loaded(t, f)
	_, cmd := enter(m)
	if msg := cmd(); msg != (MergedMsg{}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{}: leaving the queue leaves the pull request open", msg)
	}
	if f.offCall != 1 {
		t.Errorf("DisableAutoMerge calls = %d, want 1", f.offCall)
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing: the popup offers the cancellation, not the merge", f.merged)
	}
}

func TestAQueuedMergeCanBeCancelledEvenWhenMergingIsBlocked(t *testing.T) {
	t.Parallel()

	stuck := mergeable()
	stuck.Mergeable = gh.MergeableConflicting
	stuck.AutoMergeAllowed = true
	stuck.ViewerCanEnableAutoMerge = true
	stuck.AutoMergeEnabled = true
	f := &fakeSource{ctx: stuck}
	m := loaded(t, f)
	_, cmd := enter(m)
	if cmd == nil {
		t.Fatal("enter sent nothing: leaving the queue is not the merge that is blocked")
	}
	_ = cmd()
	if f.offCall != 1 {
		t.Errorf("DisableAutoMerge calls = %d, want 1", f.offCall)
	}
}

func TestSpaceDoesNothingWhenAutoMergeIsAlreadyOn(t *testing.T) {
	t.Parallel()

	on := mergeable()
	on.AutoMergeAllowed = true
	on.ViewerCanEnableAutoMerge = true
	on.AutoMergeEnabled = true
	f := &fakeSource{ctx: on}
	m := loaded(t, f)
	m, _ = press(m, " ")
	_, cmd := enter(m)
	_ = cmd()
	if f.offCall != 1 {
		t.Errorf("DisableAutoMerge calls = %d, want 1: space must not turn enter into something else", f.offCall)
	}
}

func TestRFetchesAgain(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: mergeable()}
	m := loaded(t, f)
	_, cmd := press(m, "r")
	if cmd == nil {
		t.Fatal("r returned no command: nothing refetched")
	}
	if _, ok := cmd().(contextMsg); !ok {
		t.Errorf("cmd() = %T, want contextMsg", cmd())
	}
}

func TestAFetchThatFailsIsHandedToTheHolder(t *testing.T) {
	t.Parallel()

	f := &fakeSource{err: errors.New("gh: not found")}
	m := New(f, ref())
	msg := m.Init()()
	e, ok := msg.(ErrorMsg)
	if !ok || e.Err == nil {
		t.Fatalf("Init()() = %#v, want an ErrorMsg carrying the failure", msg)
	}
}

func TestEscAsksTheHolderToTakeThePopupAway(t *testing.T) {
	t.Parallel()

	m := loaded(t, &fakeSource{ctx: mergeable()})
	_, cmd := escape(m)
	if cmd == nil {
		t.Fatal("esc returned no command: nothing closes the popup")
	}
	if msg := cmd(); msg != tea.Msg(CancelledMsg{}) {
		t.Errorf("cmd() = %#v, want CancelledMsg{}", msg)
	}
}

// TestEveryKeyIsIgnoredWhileTheMergeIsInFlight: the mutation cannot be taken
// back, so a second enter must not send a second one, and nothing may change
// what the first one was sent with.
func TestEveryKeyIsIgnoredWhileTheMergeIsInFlight(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: autoMergeable()}
	m := loaded(t, f)
	m, _ = press(m, "j")
	sending, _ := enter(m) // the command it returned is deliberately left unrun

	for _, k := range []string{"j", "k", " ", "r"} {
		got, cmd := press(sending, k)
		if cmd != nil {
			t.Errorf("%q returned %T while the merge was in flight", k, cmd())
		}
		if got.row != sending.row || got.auto != sending.auto || got.loading != sending.loading {
			t.Errorf("%q changed the popup while the merge was in flight", k)
		}
	}
	if _, cmd := escape(sending); cmd != nil {
		t.Errorf("esc returned %T while the merge was in flight", cmd())
	}
	if _, cmd := enter(sending); cmd != nil {
		t.Errorf("enter sent a second %T while the first was in flight", cmd())
	}
	if len(f.merged) != 0 || len(f.enabled) != 0 || f.offCall != 0 {
		t.Errorf("something was sent while the merge was in flight: merged=%v enabled=%v off=%d",
			f.merged, f.enabled, f.offCall)
	}
}

// TestAnAnswerForAnotherPullRequestIsDropped: the detail view hands the popup
// every message it cannot name, and a fetch the user has moved on from must
// not redraw the popup with another pull request's answer.
func TestAnAnswerForAnotherPullRequestIsDropped(t *testing.T) {
	t.Parallel()

	m := loaded(t, &fakeSource{ctx: mergeable()})
	other := gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 62}
	m, _ = m.Update(contextMsg{ref: other, gen: m.gen, ctx: gh.MergeContext{
		PullRequestID: "PR_2",
		Methods:       []gh.MergeMethod{gh.MergeRebase},
	}})
	if m.ctx.PullRequestID != "PR_1" {
		t.Errorf("PullRequestID = %q, want PR_1: the popup took another pull request's answer",
			m.ctx.PullRequestID)
	}
}

// TestTheAnswerToAReplacedFetchIsDropped: r can be pressed while the fetch
// before it is still out. The older answer would move the cursor off the
// method the user chose, and enter would then merge by a method nobody
// picked.
func TestTheAnswerToAReplacedFetchIsDropped(t *testing.T) {
	t.Parallel()

	only := mergeable()
	only.Methods = []gh.MergeMethod{gh.MergeRebase}
	f := &fakeSource{seq: []gh.MergeContext{mergeable(), mergeable(), only}}
	m := loaded(t, f)

	m, first := press(m, "r")
	m, second := press(m, "r")
	m, _ = m.Update(second()) // the newer fetch answers first
	m, _ = press(m, "j")
	m, _ = m.Update(first()) // and the older one lands after it

	_, cmd := enter(m)
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	_ = cmd()
	if len(f.merged) != 1 || f.merged[0] != gh.MergeCommit {
		t.Errorf("merged = %v, want one MergeCommit: a stale answer moved the cursor", f.merged)
	}
}

// TestRKeepsWhatTheUserChose: the fetch lives in the popup so that asking
// again does not throw the choice away.
func TestRKeepsWhatTheUserChose(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: autoMergeable()}
	m := loaded(t, f)
	m, _ = press(m, "j")
	m, _ = press(m, " ")
	m, cmd := press(m, "r")
	m, _ = m.Update(cmd())

	_, cmd = enter(m)
	_ = cmd()
	if len(f.enabled) != 1 || f.enabled[0] != gh.MergeCommit {
		t.Errorf("enabled = %v, want one MergeCommit: r threw away the method and the box", f.enabled)
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing: the auto-merge box was ticked", f.merged)
	}
}

// TestAnAutoMergeChangeKeepsThePopupUp: only the merge itself leaves nothing
// to read. Joining or leaving the queue leaves the pull request open, so the
// popup asks GitHub what it looks like now.
func TestAnAutoMergeChangeKeepsThePopupUp(t *testing.T) {
	t.Parallel()

	on := autoMergeable()
	on.AutoMergeEnabled = true
	f := &fakeSource{ctx: on}
	m := loaded(t, f)
	_, cmd := enter(m)
	msg := cmd()
	if msg != tea.Msg(MergedMsg{}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{}: the pull request was not merged", msg)
	}

	m, cmd = m.Update(msg)
	if cmd == nil {
		t.Fatal("nothing was refetched: the popup would show the queue it had before")
	}
	if !m.loading || m.sending {
		t.Errorf("loading/sending = %v/%v, want the popup waiting on its new answer", m.loading, m.sending)
	}
	m, _ = m.Update(cmd())
	if m.loading {
		t.Error("the popup dropped the answer to the refetch it asked for")
	}
}

// TestAStaleFailureIsNotThePopupsOwn: a popup that was closed and opened
// again leaves the first one's request in flight. Its failure must not clear
// the second popup's loading flag, and the holder must be able to tell.
func TestAStaleFailureIsNotThePopupsOwn(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: mergeable(), err: errors.New("gh: HTTP 500")}
	closed := New(f, ref())
	stale, ok := closed.Init()().(ErrorMsg)
	if !ok {
		t.Fatal("the fetch did not fail")
	}

	m := New(f, ref())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if m.Owns(stale) {
		t.Error("Owns says the closed popup's failure is this one's")
	}
	m, _ = m.Update(stale)
	if !m.loading {
		t.Error("the popup stopped waiting on the answer to its own fetch")
	}
}
