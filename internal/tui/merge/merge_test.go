package merge

import (
	"context"
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	ctx     gh.MergeContext
	err     error
	merged  []gh.MergeMethod
	enabled []gh.MergeMethod
	offCall int
}

func (f *fakeSource) PRMergeContext(context.Context, string, int) (gh.MergeContext, error) {
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
	if msg := cmd(); msg != (MergedMsg{}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{}", msg)
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
		t.Fatalf("cmd() = %#v, want MergedMsg{}", msg)
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
		t.Fatalf("cmd() = %#v, want MergedMsg{}", msg)
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
