package review

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	event    gh.ReviewEvent
	body     string
	calls    int
	newCalls int
	err      error
}

func (f *fakeSource) SubmitReview(_ string, event gh.ReviewEvent, body string) error {
	f.calls++
	f.event, f.body = event, body
	return f.err
}

func (f *fakeSource) SubmitNewReview(_ string, event gh.ReviewEvent, body string) error {
	f.newCalls++
	f.event, f.body = event, body
	return f.err
}

func open(src Source) Model {
	m := New(src, Target{PullRequestID: "PR_1", PendingID: "PRR_9", PendingComments: 2})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

// openWithNothingWaiting is the commonest review there is: read the diff, had
// nothing to say, approve.
func openWithNothingWaiting(src Source) Model {
	m := New(src, Target{PullRequestID: "PR_1"})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

func TestTabWalksTheThreeEvents(t *testing.T) {
	m := open(&fakeSource{})
	want := []gh.ReviewEvent{gh.EventComment, gh.EventApprove, gh.EventRequestChanges, gh.EventComment}
	if m.event != want[0] {
		t.Fatalf("the popup opens on %v, want comment", m.event)
	}
	for _, w := range want[1:] {
		m, _ = m.Update(keyPress("tab"))
		if m.event != w {
			t.Errorf("tab reached %v, want %v", m.event, w)
		}
	}
}

func TestSubmitSendsTheChosenEventAndBody(t *testing.T) {
	src := &fakeSource{}
	m := open(src)
	m, _ = m.Update(keyPress("tab")) // approve
	m = typeInto(m, "looks good")
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if src.calls != 1 {
		t.Fatalf("%d submissions, want 1", src.calls)
	}
	if src.event != gh.EventApprove {
		t.Errorf("event = %v, want approve", src.event)
	}
	if src.body != "looks good" {
		t.Errorf("body = %q", src.body)
	}
}

// TestApprovingWithNoNoteIsAllowed: an approval usually has nothing to say,
// and refusing to send one without a note would make the common case the
// awkward one.
func TestApprovingWithNoNoteIsAllowed(t *testing.T) {
	src := &fakeSource{}
	m := open(src)
	m, _ = m.Update(keyPress("tab"))
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)
	if src.calls != 1 {
		t.Errorf("%d submissions, want an approval with no note to go through", src.calls)
	}
}

// TestApprovingWithNothingWaitingGoesInOneCall: with no pending review there
// is nothing to submit against, so the review is created with its event on
// it. Going through StartReview first would leave an empty pending review
// behind if the submission then failed.
func TestApprovingWithNothingWaitingGoesInOneCall(t *testing.T) {
	src := &fakeSource{}
	m := openWithNothingWaiting(src)
	m, _ = m.Update(keyPress("tab")) // approve
	_, cmd := m.Update(keyPress("ctrl+s"))
	runCmd(t, cmd)

	if src.newCalls != 1 {
		t.Errorf("%d one-shot submissions, want 1", src.newCalls)
	}
	if src.calls != 0 {
		t.Errorf("%d submissions against a review that does not exist", src.calls)
	}
	if src.event != gh.EventApprove {
		t.Errorf("event = %v, want approve", src.event)
	}
}

func TestTheLineCommentCountIsShown(t *testing.T) {
	out := ansi.Strip(open(&fakeSource{}).View())
	if !strings.Contains(out, "2 line comments") {
		t.Errorf("the popup does not say how many comments go with it:\n%s", out)
	}
}

func TestEscCancelsWithoutSending(t *testing.T) {
	src := &fakeSource{}
	m := open(src)
	_, cmd := m.Update(keyPress("esc"))
	if src.calls != 0 {
		t.Errorf("esc submitted the review")
	}
	if _, ok := cmd().(CancelledMsg); !ok {
		t.Errorf("esc produced %T, want CancelledMsg", cmd())
	}
}
