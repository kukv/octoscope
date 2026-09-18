package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// GitHub returns an unsubmitted comment in the same place as everyone
// else's, so a thread that is public apart from one unsent reply is still
// pending.
func TestAThreadWithOneUnsentReplyIsPending(t *testing.T) {
	t.Parallel()

	thread := domain.ReviewThread{Comments: []domain.ThreadComment{
		{Body: "someone else", Pending: false},
		{Body: "mine, not sent", Pending: true},
	}}

	if !thread.Pending() {
		t.Error("a thread holding an unsent reply says it is not pending")
	}
}

func TestAThreadWithNothingUnsentIsNotPending(t *testing.T) {
	t.Parallel()

	thread := domain.ReviewThread{Comments: []domain.ThreadComment{
		{Body: "someone else"},
		{Body: "and a reply"},
	}}

	if thread.Pending() {
		t.Error("a thread with no unsent comment says it is pending")
	}

	var empty domain.ReviewThread
	if empty.Pending() {
		t.Error("a thread with no comments at all says it is pending")
	}
}

// A settled conversation is drawn as a count so that it does not push the
// code it was about off the screen. Either reason is enough on its own.
func TestEitherReasonCollapsesAThread(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		thread   domain.ReviewThread
		collapse bool
	}{
		{"still open", domain.ReviewThread{}, false},
		{"resolved", domain.ReviewThread{Resolved: true}, true},
		{"outdated", domain.ReviewThread{Outdated: true}, true},
		{"both", domain.ReviewThread{Resolved: true, Outdated: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.thread.Collapsed(); got != tt.collapse {
				t.Errorf("Collapsed() = %v, want %v", got, tt.collapse)
			}
		})
	}
}
