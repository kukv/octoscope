package merge

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/i18n"
)

type fakeSource struct {
	ctx domain.MergeContext
	// seq is one answer per fetch, in the order the fetches are run. It is
	// what tells a stale answer from a fresh one; without it every fetch
	// answers the same and the two are indistinguishable.
	seq     []domain.MergeContext
	fetches int
	err     error
	merged  []domain.MergeMethod
	enabled []domain.MergeMethod
	offCall int
}

func (f *fakeSource) PRMergeContext(context.Context, string, int) (domain.MergeContext, error) {
	i := f.fetches
	f.fetches++
	if i < len(f.seq) {
		return f.seq[i], f.err
	}
	return f.ctx, f.err
}

func (f *fakeSource) MergePR(_ domain.PullRequestHandle, m domain.MergeMethod) error {
	f.merged = append(f.merged, m)
	return nil
}

func (f *fakeSource) EnableAutoMerge(_ domain.PullRequestHandle, m domain.MergeMethod) error {
	f.enabled = append(f.enabled, m)
	return nil
}

func (f *fakeSource) DisableAutoMerge(domain.PullRequestHandle) error {
	f.offCall++
	return nil
}

func ref() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 61}
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

func autoMergeable() domain.MergeContext {
	c := mergeable()
	c.AutoMergeAllowed = true
	c.ViewerCanEnableAutoMerge = true
	return c
}

func mergeable() domain.MergeContext {
	return domain.MergeContext{
		PullRequest: "PR_1",
		Mergeable:   domain.MergeableYes,
		State:       domain.MergeStateUnstable,
		Methods:     []domain.MergeMethod{domain.MergeSquash, domain.MergeCommit, domain.MergeRebase},
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
	if len(f.merged) != 1 || f.merged[0] != domain.MergeCommit {
		t.Errorf("merged = %v, want one MergeCommit (j moved off squash)", f.merged)
	}
}

func TestTheCursorStaysInsideWhatTheRepositoryAllows(t *testing.T) {
	t.Parallel()

	only := mergeable()
	only.Methods = []domain.MergeMethod{domain.MergeRebase}
	f := &fakeSource{ctx: only}
	m := loaded(t, f)
	m, _ = press(m, "j")
	m, _ = press(m, "j")
	_, cmd := enter(m)
	_ = cmd()
	if len(f.merged) != 1 || f.merged[0] != domain.MergeRebase {
		t.Errorf("merged = %v, want one MergeRebase: j must not walk past the only method", f.merged)
	}
}

func TestEnterIsRefusedWhileGitHubIsStillWorkingItOut(t *testing.T) {
	t.Parallel()

	computing := mergeable()
	computing.Mergeable = domain.MergeableUnknown
	computing.State = domain.MergeStateUnknown
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
	clean.State = domain.MergeStateClean
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
	stuck.Mergeable = domain.MergeableConflicting
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
	other := domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 62}
	m, _ = m.Update(contextMsg{ref: other, gen: m.gen, ctx: domain.MergeContext{
		PullRequest: "PR_2",
		Methods:     []domain.MergeMethod{domain.MergeRebase},
	}})
	if m.ctx.PullRequest != "PR_1" {
		t.Errorf("PullRequest = %q, want PR_1: the popup took another pull request's answer",
			m.ctx.PullRequest)
	}
}

// TestTheAnswerToAReplacedFetchIsDropped: r can be pressed while the fetch
// before it is still out. The older answer would move the cursor off the
// method the user chose, and enter would then merge by a method nobody
// picked.
func TestTheAnswerToAReplacedFetchIsDropped(t *testing.T) {
	t.Parallel()

	only := mergeable()
	only.Methods = []domain.MergeMethod{domain.MergeRebase}
	f := &fakeSource{seq: []domain.MergeContext{mergeable(), mergeable(), only}}
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
	if len(f.merged) != 1 || f.merged[0] != domain.MergeCommit {
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
	if len(f.enabled) != 1 || f.enabled[0] != domain.MergeCommit {
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

// blocked is a pull request a branch rule is holding, with the viewer able
// to push past it.
func blocked() domain.MergeContext {
	c := mergeable()
	c.State = domain.MergeStateBlocked
	c.ViewerIsAdmin = true
	return c
}

func TestAPressedOnABlockedPullRequestMergesIt(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: blocked()}
	m := loaded(t, f)
	_, cmd := press(m, "a")
	if cmd == nil {
		t.Fatal("a returned no command: nothing was sent")
	}
	if msg := cmd(); msg != (MergedMsg{Merged: true}) {
		t.Fatalf("cmd() = %#v, want MergedMsg{Merged: true}", msg)
	}
	if len(f.merged) != 1 || f.merged[0] != domain.MergeSquash {
		t.Errorf("merged = %v, want one MergeSquash (the method on the cursor)", f.merged)
	}
}

// The key that breaks a protection must never be the key an ordinary merge
// is sent with: a mistake has to be a wrong key, not the usual one.
func TestEnterStillSendsNothingOnABlockedPullRequest(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: blocked()}
	m := loaded(t, f)
	_, cmd := enter(m)
	if cmd != nil {
		t.Fatal("enter returned a command: a blocked merge must still be refused")
	}
	if len(f.merged) != 0 {
		t.Errorf("merged = %v, want nothing", f.merged)
	}
}

func TestAIsRefusedWhereNoPermissionWouldHelp(t *testing.T) {
	t.Parallel()

	draft := blocked()
	draft.IsDraft = true

	conflicting := blocked()
	conflicting.Mergeable = domain.MergeableConflicting
	conflicting.State = domain.MergeStateDirty

	computing := blocked()
	computing.Mergeable = domain.MergeableUnknown
	computing.State = domain.MergeStateUnknown

	notAdmin := blocked()
	notAdmin.ViewerIsAdmin = false

	tests := []struct {
		name string
		ctx  domain.MergeContext
	}{
		{"draft", draft},
		{"conflicting", conflicting},
		{"computing", computing},
		{"the viewer is no admin", notAdmin},
		{"nothing is holding it: enter is the key for that", mergeable()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeSource{ctx: tt.ctx}
			m := loaded(t, f)
			_, cmd := press(m, "a")
			if cmd != nil {
				t.Error("a returned a command, want nothing sent")
			}
			if len(f.merged) != 0 {
				t.Errorf("merged = %v, want nothing", f.merged)
			}
		})
	}
}

func TestAIsIgnoredWhileAMergeIsInFlight(t *testing.T) {
	t.Parallel()

	f := &fakeSource{ctx: blocked()}
	m := loaded(t, f)
	m, cmd := press(m, "a") // the command is deliberately left unrun
	if cmd == nil {
		t.Fatal("a returned no command: the test needs the popup mid-send")
	}
	_, cmd = press(m, "a")
	if cmd != nil {
		t.Error("a returned a second command while the first was in flight")
	}
}

// A key that works but is not on the bar is a key nobody finds.
func TestTheBlockedPopupOffersTheAdminKey(t *testing.T) {
	t.Parallel()

	m := loaded(t, &fakeSource{ctx: blocked()})
	view := ansi.Strip(m.View())
	if want := i18n.T("merge.key_admin"); !strings.Contains(view, want) {
		t.Errorf("the key bar has no %q:\n%s", want, view)
	}
	if want := i18n.T("merge.admin_offer"); !strings.Contains(view, want) {
		t.Errorf("the popup does not say %q:\n%s", want, view)
	}
	if notWant := i18n.T("merge.key_merge"); strings.Contains(view, notWant) {
		t.Errorf("the key bar offers %q on a blocked pull request:\n%s", notWant, view)
	}
}

func TestAPopupWithNoAdminOfferSaysNothingAboutIt(t *testing.T) {
	t.Parallel()

	c := blocked()
	c.ViewerIsAdmin = false
	m := loaded(t, &fakeSource{ctx: c})
	view := ansi.Strip(m.View())
	if notWant := i18n.T("merge.key_admin"); strings.Contains(view, notWant) {
		t.Errorf("the key bar offers %q to a viewer who may not:\n%s", notWant, view)
	}
	if notWant := i18n.T("merge.admin_offer"); strings.Contains(view, notWant) {
		t.Errorf("the popup says %q to a viewer who may not:\n%s", notWant, view)
	}
}
