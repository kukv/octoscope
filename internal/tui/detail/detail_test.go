package detail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/usecase"
)

// fakeSource implements Source and records calls.
type fakeSource struct {
	pr       gh.PR
	issue    gh.Issue
	err      error
	webCalls []string // the URLs handed to the browser

	commentCalls []string // "pr:<repo>:<n>:<body>" / "issue:<repo>:<n>:<body>"
	commentErr   error

	stateCalls []string // action:kind:repo:number, e.g. "close:pr::1"
	stateErr   error

	labels    []gh.Label
	users     []string
	editCalls []string // "pr:labels::1:add=bug:remove=wip"
	labelsErr error
	usersErr  error
	editErr   error

	reviewCtx gh.ReviewContext
	reviewErr error

	submitCalls []string // "<pending id>:<body>"
	submitErr   error
}

func (f *fakeSource) GetItem(_ context.Context, ref gh.ItemRef) (usecase.Item, error) {
	if ref.Kind == gh.ItemPR {
		return prItem(f.pr), f.err
	}
	return issueItem(f.issue), f.err
}

func prItem(pr gh.PR) usecase.Item {
	return usecase.Item{
		Kind: gh.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
		State: pr.State, Body: pr.Body, URL: pr.URL, Labels: pr.Labels,
		Assignees: pr.Assignees, Comments: pr.Comments, UpdatedAt: pr.UpdatedAt,
		PR: &pr,
	}
}

func issueItem(issue gh.Issue) usecase.Item {
	return usecase.Item{
		Kind: gh.ItemIssue, Number: issue.Number, Title: issue.Title, Author: issue.Author,
		State: issue.State, Body: issue.Body, URL: issue.URL, Labels: issue.Labels,
		Assignees: issue.Assignees, Comments: issue.Comments, UpdatedAt: issue.UpdatedAt,
	}
}

func kindName(ref gh.ItemRef) string {
	if ref.Kind == gh.ItemPR {
		return "pr"
	}
	return "issue"
}

func (f *fakeSource) OpenWeb(url string) error {
	f.webCalls = append(f.webCalls, url)
	return nil
}

func (f *fakeSource) AddComment(ref gh.ItemRef, body string) error {
	f.commentCalls = append(f.commentCalls, kindName(ref)+":"+ref.Repo+":"+itoa(ref.Number)+":"+body)
	return f.commentErr
}

func (f *fakeSource) SetState(ref gh.ItemRef, closing bool) error {
	action := "reopen"
	if closing {
		action = "close"
	}
	f.stateCalls = append(f.stateCalls, action+":"+kindName(ref)+":"+ref.Repo+":"+itoa(ref.Number))
	return f.stateErr
}

func (f *fakeSource) ListLabels(ctx context.Context, repo string) ([]gh.Label, error) {
	return f.labels, f.labelsErr
}

func (f *fakeSource) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	return f.users, f.usersErr
}

func (f *fakeSource) EditLabels(ref gh.ItemRef, add, remove []string) error {
	f.editCalls = append(f.editCalls, kindName(ref)+":labels:"+ref.Repo+":"+itoa(ref.Number)+editSuffix(add, remove))
	return f.editErr
}

func (f *fakeSource) EditAssignees(ref gh.ItemRef, add, remove []string) error {
	f.editCalls = append(f.editCalls, kindName(ref)+":assignees:"+ref.Repo+":"+itoa(ref.Number)+editSuffix(add, remove))
	return f.editErr
}

func (f *fakeSource) PRReviewContext(ctx context.Context, repo string, n int) (gh.ReviewContext, error) {
	return f.reviewCtx, f.reviewErr
}

func (f *fakeSource) SubmitReview(t usecase.ReviewTarget, event gh.ReviewEvent, body string) error {
	f.submitCalls = append(f.submitCalls, t.PendingID+":"+body)
	return f.submitErr
}

func editSuffix(add, remove []string) string {
	return ":add=" + strings.Join(add, ",") + ":remove=" + strings.Join(remove, ",")
}

func itoa(n int) string { return string(rune('0' + n)) } // tests only use n < 10

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

func prRef() gh.ItemRef    { return gh.ItemRef{Kind: gh.ItemPR, Number: 1} }
func issueRef() gh.ItemRef { return gh.ItemRef{Kind: gh.ItemIssue, Number: 5} }

// loaded returns a model whose item has already arrived. It runs the fetch
// directly rather than through Init, whose batch also holds the spinner tick
// (a command that sleeps a frame before it reports); TestInitStartsTheFetch
// covers Init itself.
func loaded(f *fakeSource, ref gh.ItemRef) Model {
	m := New(f, ref)
	m, _ = m.Update(fetch(f, ref)())
	return m
}

// initFetch picks the fetch out of the batch Init returns.
func initFetch(t *testing.T, m Model) tea.Cmd {
	t.Helper()
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init = %T, want a batch of the spinner tick and the fetch", m.Init()())
	}
	if len(batch) != 2 {
		t.Fatalf("Init batched %d commands, want the spinner tick and the fetch", len(batch))
	}
	return batch[1]
}

func TestInitStartsTheFetch(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := New(f, prRef())
	if _, ok := initFetch(t, m)().(itemMsg); !ok {
		t.Errorf("the batched fetch did not produce an itemMsg")
	}
}

func TestDetailRendersBodyAndComments(t *testing.T) {
	f := &fakeSource{pr: gh.PR{
		Number: 1, Title: "first pr", Author: gh.Author{Login: "kukv"},
		Body: "the body text", Comments: []gh.Comment{
			{Author: gh.Author{Login: "bob"}, Body: "a comment"},
		},
	}}
	m := loaded(f, prRef())
	// glamour v2 styles per cell, so the body is checked with the ANSI stripped.
	view := ansi.Strip(m.View())
	for _, want := range []string{"first pr", "the body text", "a comment"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view missing %q:\n%s", want, view)
		}
	}
}

func TestEscAndQCloseTheView(t *testing.T) {
	for _, k := range []string{"esc", "q"} {
		t.Run(k, func(t *testing.T) {
			f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
			m := loaded(f, prRef())
			_, cmd := m.Update(key(k))
			if cmd == nil {
				t.Fatalf("cmd = nil after %s, want ClosedMsg cmd", k)
			}
			if _, ok := cmd().(ClosedMsg); !ok {
				t.Errorf("msg = %T, want ClosedMsg", cmd())
			}
		})
	}
}

func TestFetchFailureBecomesErrorMsg(t *testing.T) {
	f := &fakeSource{err: errors.New("gh pr: no git remotes found")}
	m := New(f, prRef())
	m, cmd := m.Update(initFetch(t, m)())
	if m.phase != phaseIdle {
		t.Errorf("phase = %v after a failed fetch, want the wait to be over", m.phase)
	}
	if cmd == nil {
		t.Fatal("cmd = nil after a failed fetch, want ErrorMsg cmd")
	}
	msg, ok := cmd().(ErrorMsg)
	if !ok {
		t.Fatalf("msg = %T, want ErrorMsg", cmd())
	}
	if !strings.Contains(msg.Err.Error(), "no git remotes found") {
		t.Errorf("Err = %v, want the source's error", msg.Err)
	}
}

func TestDAsksForTheDiff(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	_, cmd := m.Update(key("d"))
	if cmd == nil {
		t.Fatal("d produced no command")
	}
	msg, ok := cmd().(OpenDiffMsg)
	if !ok {
		t.Fatalf("got %T, want OpenDiffMsg", cmd())
	}
	if msg.Ref != prRef() {
		t.Errorf("Ref = %+v, want %+v", msg.Ref, prRef())
	}
}

// TestDDoesNothingOnAnIssue is what stops the diff view opening on something
// that has no diff.
func TestDDoesNothingOnAnIssue(t *testing.T) {
	f := &fakeSource{issue: gh.Issue{Number: 5, Title: "an issue"}}
	m := loaded(f, issueRef())
	if _, cmd := m.Update(key("d")); cmd != nil {
		t.Errorf("d on an issue produced %T", cmd())
	}
}

// TestOOpensTheShownItemsOwnURL pins that o opens the address GitHub gave
// the item, rather than one octoscope spelled out itself.
func TestOOpensTheShownItemsOwnURL(t *testing.T) {
	const want = "https://github.com/kukv/demo/pull/1"
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", URL: want}}
	m := loaded(f, prRef())
	_, cmd := m.Update(key("o"))
	if cmd == nil {
		t.Fatal("cmd = nil, want openWeb cmd")
	}
	cmd()
	if len(f.webCalls) != 1 || f.webCalls[0] != want {
		t.Errorf("webCalls = %v, want [%s]", f.webCalls, want)
	}
}

// TestODoesNothingBeforeTheItemLands guards the other half: with no item
// there is no URL, and opening the empty string would take the browser
// nowhere.
func TestODoesNothingBeforeTheItemLands(t *testing.T) {
	m := New(&fakeSource{}, prRef())
	if _, cmd := m.Update(key("o")); cmd != nil {
		t.Errorf("o before the item arrived produced %T", cmd())
	}
}

func TestDetailRefetchesOnR(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("r"))
	if m.phase != phaseLoading || cmd == nil {
		t.Errorf("phase = %v, cmd = %v; want a fetch in flight", m.phase, cmd)
	}
}

func TestCEntersCompose(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	if m.mode != modeCompose {
		t.Errorf("mode = %v, want the composer after c", m.mode)
	}
}

func TestComposeEmptyBodyNotSent(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	m, cmd := m.Update(key("ctrl+s")) // the textarea is empty
	if cmd != nil {
		t.Errorf("cmd = non-nil, want nil for empty body")
	}
	if m.mode != modeCompose {
		t.Errorf("mode = %v, want the composer to stay open", m.mode)
	}
	if len(f.commentCalls) != 0 {
		t.Errorf("commentCalls = %v, want none", f.commentCalls)
	}
}

func TestComposeSubmitPostsAndRefetches(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	m.textarea.SetValue("looks good")
	m, cmd := m.Update(key("ctrl+s"))
	if m.phase != phaseWorking || cmd == nil {
		t.Fatalf("phase = %v, cmd = %v; want the post in flight", m.phase, cmd)
	}
	msg := cmd()
	if _, ok := msg.(commentPostedMsg); !ok {
		t.Fatalf("msg = %T, want commentPostedMsg", msg)
	}
	if len(f.commentCalls) != 1 || f.commentCalls[0] != "pr::1:looks good" {
		t.Fatalf("commentCalls = %v, want [pr::1:looks good]", f.commentCalls)
	}
	m, cmd = m.Update(msg)
	if m.mode != modeView || m.phase != phaseLoading || cmd == nil {
		t.Errorf("after posted: mode/phase = %v/%v, cmd = %v; want the body reloading",
			m.mode, m.phase, cmd)
	}
}

func TestComposeEscCancels(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	m.textarea.SetValue("draft")
	m, cmd := m.Update(key("esc"))
	if m.mode != modeView {
		t.Errorf("mode = %v after esc, want the body back", m.mode)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil after esc, want nil (esc cancels compose, it does not close the view)")
	}
}

func TestComposePostErrorKeepsDraft(t *testing.T) {
	f := &fakeSource{
		pr:         gh.PR{Number: 1, Title: "first pr"},
		commentErr: errors.New("gh pr: HTTP 403 forbidden"),
	}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	m.textarea.SetValue("hello")
	m, cmd := m.Update(key("ctrl+s"))
	m, _ = m.Update(cmd()) // commentErrorMsg
	if m.mode != modeCompose || m.phase != phaseIdle {
		t.Errorf("mode/phase = %v/%v, want the composer open and no longer sending",
			m.mode, m.phase)
	}
	if !strings.Contains(m.errText, "403") {
		t.Errorf("errText = %q, want to contain 403", m.errText)
	}
	if m.textarea.Value() != "hello" {
		t.Errorf("draft lost: textarea = %q, want hello", m.textarea.Value())
	}
}

func TestComposeViewShowsTextareaAndHelp(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	m.textarea.SetValue("my comment")
	view := m.View()
	for _, want := range []string{"my comment", "ctrl+s:send", "esc:cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("compose view missing %q:\n%s", want, view)
		}
	}
}

func TestComposeViewShowsPostError(t *testing.T) {
	f := &fakeSource{
		pr:         gh.PR{Number: 1, Title: "first pr"},
		commentErr: errors.New("gh pr: HTTP 403 forbidden"),
	}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	m.textarea.SetValue("hello")
	m, cmd := m.Update(key("ctrl+s"))
	m, _ = m.Update(cmd())
	if !strings.Contains(m.View(), "403") {
		t.Errorf("compose view missing error text:\n%s", m.View())
	}
}

func TestComposeIgnoresKeysWhilePosting(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("c"))
	m.textarea.SetValue("hello")
	m, _ = m.Update(key("ctrl+s")) // the post cmd is deliberately not run
	if m.phase != phaseWorking {
		t.Fatalf("precondition: phase = %v, want the post in flight", m.phase)
	}
	m, cmd := m.Update(key("esc"))
	if cmd != nil {
		t.Errorf("cmd = non-nil while posting, want nil")
	}
	if m.mode != modeCompose || m.phase != phaseWorking {
		t.Errorf("mode/phase = %v/%v, want compose/working while the post is in flight",
			m.mode, m.phase)
	}
	if m.textarea.Value() != "hello" {
		t.Errorf("draft changed while posting: %q", m.textarea.Value())
	}
}

func TestXEntersConfirmWhenOpen(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	if m.mode != modeConfirm {
		t.Errorf("mode = %v, want the confirmation after x on an open item", m.mode)
	}
}

func TestXIgnoredWhenMerged(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateMerged}}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("x"))
	if m.mode != modeView {
		t.Errorf("mode = %v, want the body untouched for a merged item", m.mode)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil, want nil for merged item")
	}
}

func TestConfirmYClosesAndRefetches(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	m, cmd := m.Update(key("y"))
	if m.phase != phaseWorking || cmd == nil {
		t.Fatalf("phase = %v, cmd = %v; want the state change in flight", m.phase, cmd)
	}
	msg := cmd()
	if _, ok := msg.(stateChangedMsg); !ok {
		t.Fatalf("msg = %T, want stateChangedMsg", msg)
	}
	if len(f.stateCalls) != 1 || f.stateCalls[0] != "close:pr::1" {
		t.Fatalf("stateCalls = %v, want [close:pr::1]", f.stateCalls)
	}
	m, cmd = m.Update(msg)
	if m.mode != modeView || m.phase != phaseLoading || cmd == nil {
		t.Errorf("after changed: mode/phase = %v/%v, cmd = %v; want the body reloading",
			m.mode, m.phase, cmd)
	}
}

func TestConfirmReopenRoutesToReopen(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateClosed}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	_, cmd := m.Update(key("y"))
	if cmd == nil {
		t.Fatal("cmd = nil, want reopen cmd")
	}
	if _, ok := cmd().(stateChangedMsg); !ok {
		t.Fatalf("msg = %T, want stateChangedMsg", cmd())
	}
	if len(f.stateCalls) != 1 || f.stateCalls[0] != "reopen:pr::1" {
		t.Errorf("stateCalls = %v, want [reopen:pr::1]", f.stateCalls)
	}
}

func TestConfirmNCancels(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	m, cmd := m.Update(key("n"))
	if m.mode != modeView {
		t.Errorf("mode = %v after n, want the body back", m.mode)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil after n, want nil")
	}
	if len(f.stateCalls) != 0 {
		t.Errorf("stateCalls = %v, want none", f.stateCalls)
	}
}

func TestConfirmEscCancels(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	m, cmd := m.Update(key("esc"))
	if m.mode != modeView {
		t.Errorf("mode = %v after esc, want the body back", m.mode)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil after esc, want nil (esc cancels the confirmation, it does not close the view)")
	}
}

func TestStateErrorStaysOnDetail(t *testing.T) {
	f := &fakeSource{
		pr:       gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		stateErr: errors.New("gh pr: HTTP 403 forbidden"),
	}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	m, cmd := m.Update(key("y"))
	m, cmd = m.Update(cmd()) // stateErrorMsg
	if m.mode != modeView || m.phase != phaseIdle {
		t.Errorf("mode/phase = %v/%v, want the body back and nothing in flight",
			m.mode, m.phase)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil, want nil (a state failure stays inline, it is not the parent's error screen)")
	}
	if !strings.Contains(m.errText, "403") {
		t.Errorf("errText = %q, want to contain 403", m.errText)
	}
	if !strings.Contains(m.View(), "403") {
		t.Errorf("detail view missing inline error:\n%s", m.View())
	}
}

func TestConfirmViewShowsPrompt(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	view := m.View()
	for _, want := range []string{"Close", "(y/n)"} {
		if !strings.Contains(view, want) {
			t.Errorf("confirm view missing %q:\n%s", want, view)
		}
	}
}

func TestConfirmViewReopenWording(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateClosed}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	if !strings.Contains(m.View(), "Reopen") {
		t.Errorf("confirm view missing Reopen wording:\n%s", m.View())
	}
}

// TestVFetchesReviewContextAndOpensThePopup: unlike the diff view, detail
// never has a review context already on the model, so v always fetches
// first.
func TestVFetchesReviewContextAndOpensThePopup(t *testing.T) {
	f := &fakeSource{
		pr:        gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_1"},
	}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("v"))
	if m.mode != modeSubmit || m.phase != phaseLoading || cmd == nil {
		t.Fatalf("mode/phase = %v/%v, cmd = %v; want the review context in flight",
			m.mode, m.phase, cmd)
	}
	msg := cmd()
	if _, ok := msg.(reviewContextMsg); !ok {
		t.Fatalf("msg = %T, want reviewContextMsg", msg)
	}
	m, _ = m.Update(msg)
	if m.mode != modeSubmit || m.phase != phaseIdle {
		t.Errorf("mode/phase = %v/%v; want the popup open", m.mode, m.phase)
	}
}

// TestVDoesNothingOnAnIssue mirrors d's own guard: an issue has no review.
func TestVDoesNothingOnAnIssue(t *testing.T) {
	f := &fakeSource{issue: gh.Issue{Number: 5, Title: "an issue"}}
	m := loaded(f, issueRef())
	if _, cmd := m.Update(key("v")); cmd != nil {
		t.Errorf("v on an issue produced %T", cmd())
	}
}

// TestReviewContextFailureStaysInline is the same rule as a state-change
// failure: the fetch's own error is shown inline, not escalated to the
// parent's error screen.
func TestReviewContextFailureStaysInline(t *testing.T) {
	f := &fakeSource{
		pr:        gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		reviewErr: errors.New("gh pr: HTTP 403 forbidden"),
	}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("v"))
	m, cmd = m.Update(cmd())
	if m.mode != modeView {
		t.Errorf("mode = %v after a failed review-context fetch, want the body back", m.mode)
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil, want nil (a fetch failure stays inline)")
	}
	if !strings.Contains(m.errText, "403") {
		t.Errorf("errText = %q, want to contain 403", m.errText)
	}
}

// TestSubmitEscCancelsThePopup checks that esc from inside the popup takes it
// away without closing the whole view.
func TestSubmitEscCancelsThePopup(t *testing.T) {
	f := &fakeSource{
		pr:        gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_1"},
	}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("v"))
	m, _ = m.Update(cmd())
	if m.mode != modeSubmit {
		t.Fatalf("precondition: mode = %v, want the popup open", m.mode)
	}
	m, cmd = m.Update(key("esc"))
	m, cmd = m.Update(cmd())
	if m.mode != modeView {
		t.Error("esc did not close the submit popup")
	}
	if cmd != nil {
		t.Errorf("cmd = non-nil after CancelledMsg, want nil (esc cancels the popup, it does not close the view)")
	}
}

// TestSubmitSuccessRefetches checks the other exit from the popup: a
// successful submit reloads the item, so an approval's new review state
// shows up.
func TestSubmitSuccessRefetches(t *testing.T) {
	f := &fakeSource{
		pr:        gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_1"},
	}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("v"))
	m, _ = m.Update(cmd())
	m, cmd = m.Update(key("ctrl+s"))
	msg := cmd()
	m, cmd = m.Update(msg)
	if m.mode != modeView || m.phase != phaseLoading || cmd == nil {
		t.Errorf("mode/phase = %v/%v, cmd = %v; want the body reloading", m.mode, m.phase, cmd)
	}
	if len(f.submitCalls) != 1 {
		t.Errorf("submitCalls = %v, want one submission", f.submitCalls)
	}
}

func TestDetailFooterShowsStateAndPickerKeys(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	m := loaded(f, prRef())
	view := m.View()
	for _, want := range []string{"x:close", "l:labels", "a:assign"} {
		if !strings.Contains(view, want) {
			t.Errorf("detail footer missing %q:\n%s", want, view)
		}
	}
}

// TestActionErrClearedOnReload guards against a stale error surviving a
// successful reload.
func TestActionErrClearedOnReload(t *testing.T) {
	f := &fakeSource{
		pr:       gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		stateErr: errors.New("gh pr: HTTP 403 forbidden"),
	}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	m, cmd := m.Update(key("y"))
	m, _ = m.Update(cmd()) // stateErrorMsg
	if !strings.Contains(m.errText, "403") {
		t.Fatalf("precondition: errText = %q, want to contain 403", m.errText)
	}

	m, cmd = m.Update(key("r"))
	if m.phase != phaseLoading || cmd == nil {
		t.Fatalf("phase = %v, cmd = %v; want a fetch in flight", m.phase, cmd)
	}
	m, _ = m.Update(cmd())
	if m.errText != "" {
		t.Errorf("errText = %q after reload, want empty", m.errText)
	}
	if strings.Contains(m.View(), "403") {
		t.Errorf("view still shows stale error after reload:\n%s", m.View())
	}
}

// TestOpeningTheComposerTakesTheBodysErrorWithIt pins what one error string
// costs: a failed close leaves its text under the body, and c clears it
// rather than draw it inside the composer.
func TestOpeningTheComposerTakesTheBodysErrorWithIt(t *testing.T) {
	f := &fakeSource{
		pr:       gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		stateErr: errors.New("gh pr: HTTP 403 forbidden"),
	}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	m, cmd := m.Update(key("y"))
	m, _ = m.Update(cmd()) // stateErrorMsg
	if !strings.Contains(m.View(), "403") {
		t.Fatalf("precondition: the failed close is not on the body:\n%s", m.View())
	}

	m, _ = m.Update(key("c"))
	if strings.Contains(m.View(), "403") {
		t.Errorf("the close failure is drawn inside the composer:\n%s", m.View())
	}
	m, _ = m.Update(key("esc"))
	if strings.Contains(m.View(), "403") {
		t.Errorf("the close failure came back after leaving the composer:\n%s", m.View())
	}
}

func TestConfirmIgnoresKeysWhileWorking(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	m := loaded(f, prRef())
	m, _ = m.Update(key("x"))
	m, _ = m.Update(key("y")) // the state cmd is deliberately not run
	if m.phase != phaseWorking {
		t.Fatalf("precondition: phase = %v, want the change in flight", m.phase)
	}
	m, cmd := m.Update(key("esc"))
	if cmd != nil {
		t.Errorf("cmd = non-nil while working, want nil")
	}
	if m.mode != modeConfirm || m.phase != phaseWorking {
		t.Errorf("mode/phase = %v/%v, want confirm/working while the change is in flight",
			m.mode, m.phase)
	}
}

// spinnerFrame is the first frame of spinner.Dot, which is what a freshly
// built model draws.
const spinnerFrame = "⣾"

func TestLoadingShowsSpinnerAndText(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr"}}
	m := New(f, prRef())
	view := m.View()
	if !strings.Contains(view, "loading...") {
		t.Errorf("view missing the loading text before the fetch resolves:\n%s", view)
	}
	if !strings.Contains(view, spinnerFrame) {
		t.Errorf("view missing the spinner frame while loading:\n%s", view)
	}
}

// TestBusyStatesShowTheSpinner keeps the spinner on every screen that waits:
// the detail fetch, a comment being posted, a close being applied and a label
// edit being applied all animate, as they do in internal/ui today.
func TestBusyStatesShowTheSpinner(t *testing.T) {
	newPR := func() *fakeSource {
		return &fakeSource{
			pr:     gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen, Labels: []gh.Label{{Name: "bug"}}},
			labels: []gh.Label{{Name: "bug"}, {Name: "wip"}},
		}
	}
	cases := map[string]func(t *testing.T) Model{
		"loading": func(*testing.T) Model { return New(newPR(), prRef()) },
		"posting": func(*testing.T) Model {
			m := loaded(newPR(), prRef())
			m, _ = m.Update(key("c"))
			m.textarea.SetValue("hello")
			m, _ = m.Update(key("ctrl+s")) // the post cmd is deliberately not run
			return m
		},
		"working": func(*testing.T) Model {
			m := loaded(newPR(), prRef())
			m, _ = m.Update(key("x"))
			m, _ = m.Update(key("y")) // the state cmd is deliberately not run
			return m
		},
		"applying": func(t *testing.T) Model {
			m := openPicker(t, newPR(), prRef(), "l")
			m, _ = m.Update(key("space"))
			m, _ = m.Update(key("enter")) // the edit cmd is deliberately not run
			return m
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			if view := build(t).View(); !strings.Contains(view, spinnerFrame) {
				t.Errorf("%s view missing the spinner frame:\n%s", name, view)
			}
		})
	}
}

func TestSpinnerTickAdvancesTheFrame(t *testing.T) {
	m := New(&fakeSource{}, prRef())
	before := m.spin.View()
	m, cmd := m.Update(m.spin.Tick())
	if cmd == nil {
		t.Fatal("a tick produced no follow-up command; the animation would stop")
	}
	if m.spin.View() == before {
		t.Errorf("the spinner frame did not advance: still %q", before)
	}
}

// TestAnAnswerForAnotherItemIsDropped covers what happens when the user opens
// one item, leaves it and opens another quickly: the first request is still
// running, and its answer must not put the wrong item on the screen.
func TestAnAnswerForAnotherItemIsDropped(t *testing.T) {
	other := gh.ItemRef{Kind: gh.ItemPR, Number: 99}
	m := New(&fakeSource{}, prRef())

	next, _ := m.Update(itemMsg{other, prItem(gh.PR{Number: 99, Title: "the previous one"})})
	if next.phase != phaseLoading {
		t.Error("an answer for another item ended the wait for this one")
	}
	if next.title != "" {
		t.Errorf("the view took the other item's title: %q", next.title)
	}

	issue := New(&fakeSource{}, issueRef())
	next, _ = issue.Update(itemMsg{other, issueItem(gh.Issue{Number: 99, Title: "the previous one"})})
	if next.phase != phaseLoading || next.title != "" {
		t.Errorf("an issue answer for another item was accepted: %q", next.title)
	}
}

// TestTheWheelDoesNotScrollWhatTheSpinnerHides fixes a deliberate change:
// while the picker's candidates are in flight the view draws a spinner, not
// the body, so the wheel has nothing to move.
func TestTheWheelDoesNotScrollWhatTheSpinnerHides(t *testing.T) {
	f := &fakeSource{pr: longPR(), labels: []gh.Label{{Name: "bug"}}}
	m := loaded(f, prRef())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = m.Update(key("l")) // the candidates are now in flight

	before := m.body.YOffset()
	m, _ = m.Update(wheelDown())

	if m.body.YOffset() != before {
		t.Errorf("the body scrolled to %d while the fetch was in flight, want %d",
			m.body.YOffset(), before)
	}
}

// TestEveryStateNamesItself catches an overlay added without a name: every
// assertion in this package reports mode and phase with %v, and a nameless
// one is printed as a number nobody can read.
func TestEveryStateNamesItself(t *testing.T) {
	for m := modeView; m <= modeSubmit; m++ {
		if got := fmt.Sprintf("%v", m); strings.Contains(got, "?") {
			t.Errorf("mode %d prints as %q", uint8(m), got)
		}
	}
	for p := phaseIdle; p <= phaseWorking; p++ {
		if got := fmt.Sprintf("%v", p); strings.Contains(got, "?") {
			t.Errorf("phase %d prints as %q", uint8(p), got)
		}
	}
}
