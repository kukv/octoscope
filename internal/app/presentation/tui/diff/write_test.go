package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/review"
	"github.com/kukv/octoscope/internal/i18n"
)

// After a comment, a discard or a submit, the screen is built from what the
// write answered rather than from a refetch of every thread. Refetches fly
// independently and land in whatever order the network delivers them, so a
// slow one started by an earlier comment could overwrite a discard with the
// comments it had just deleted. These tests fail on a returned command: the
// only command any of the three used to return was that refetch.

func TestAPostedCommentIsDrawnWithoutARefetch(t *testing.T) {
	m := withThreads(t, 120, 40)
	posted := domain.ReviewThread{
		Path: "graph/walk.go", Line: 14, Side: domain.SideRight,
		Comments: []domain.ThreadComment{{Author: domain.Author{Login: "kukv"}, Body: "just posted", Pending: true}},
	}
	m, cmd := m.Update(commentPostedMsg{ref: m.ref, review: "PRR_1", thread: posted})
	if cmd != nil {
		t.Errorf("commentPostedMsg returned a command (%T), want none", cmd())
	}
	if m.mode != modeView || m.phase != phaseIdle {
		t.Errorf("mode = %v, phase = %v, want the composer closed and idle", m.mode, m.phase)
	}
	if m.review.Pending != "PRR_1" {
		t.Errorf("review.Pending = %q, want PRR_1", m.review.Pending)
	}
	out := ansi.Strip(m.View())
	for _, want := range []string{"just posted", i18n.T("diff.unsent"), "is 2 not the default?"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q is not on screen:\n%s", want, out)
		}
	}
}

func TestADiscardDropsThePendingCommentsWithoutARefetch(t *testing.T) {
	m := withThreads(t, 120, 40)
	ctx := threadFixture()
	ctx.Pending = "PRR_9"
	ctx.Threads[0].Comments = append(ctx.Threads[0].Comments,
		domain.ThreadComment{Author: domain.Author{Login: "kukv"}, Body: "my pending reply", Pending: true})
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: ctx})
	m.mode, m.phase = modeDiscard, phaseWorking

	m, cmd := m.Update(discardedMsg{ref: m.ref})
	if cmd != nil {
		t.Errorf("discardedMsg returned a command (%T), want none", cmd())
	}
	if m.review.Pending != "" {
		t.Errorf("review.Pending = %q, want none", m.review.Pending)
	}
	out := ansi.Strip(m.View())
	if strings.Contains(out, "my pending reply") {
		t.Errorf("the discarded comment is still on screen:\n%s", out)
	}
	if !strings.Contains(out, "is 2 not the default?") {
		t.Errorf("the published comment in the same thread went with it:\n%s", out)
	}
}

func TestASubmitPublishesThePendingCommentsWithoutARefetch(t *testing.T) {
	m := withPending(t)
	m.review.Pending = "PRR_9"
	m.mode = modeSubmit

	m, cmd := m.Update(review.SubmittedMsg{})
	if cmd != nil {
		t.Errorf("SubmittedMsg returned a command (%T), want none", cmd())
	}
	if m.review.Pending != "" {
		t.Errorf("review.Pending = %q, want none", m.review.Pending)
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "draft reply") {
		t.Errorf("the submitted comment is gone:\n%s", out)
	}
	if strings.Contains(out, i18n.T("diff.unsent")) {
		t.Errorf("the submitted comment is still marked as not sent:\n%s", out)
	}
}
