package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// PendingCount walks two loops -- threads, then the comments inside each --
// so the case that matters is unsubmitted comments spread across more than
// one thread. A single thread would only exercise the inner loop.
//
// The two counts are deliberately different (two unsent, three not): with an
// equal number of each, inverting the condition would still answer two and
// the test would pass on a broken sum.
func TestPendingCountAddsUpAcrossThreads(t *testing.T) {
	t.Parallel()

	c := domain.ReviewContext{Threads: []domain.ReviewThread{
		{Comments: []domain.ThreadComment{
			{Body: "someone else", Pending: false},
			{Body: "mine, not sent", Pending: true},
		}},
		{Comments: []domain.ThreadComment{
			{Body: "mine, not sent", Pending: true},
		}},
		{Comments: []domain.ThreadComment{
			{Body: "all public", Pending: false},
			{Body: "still public", Pending: false},
		}},
	}}

	if got := c.PendingCount(); got != 2 {
		t.Errorf("PendingCount() = %d, want 2", got)
	}
}

func TestPendingCountIsZeroWhenNothingIsUnsent(t *testing.T) {
	t.Parallel()

	c := domain.ReviewContext{Threads: []domain.ReviewThread{
		{Comments: []domain.ThreadComment{{Body: "public"}}},
		{Comments: []domain.ThreadComment{{Body: "public"}}},
	}}

	if got := c.PendingCount(); got != 0 {
		t.Errorf("PendingCount() = %d, want 0", got)
	}
}

// A review context arrives before its threads do, and a pull request with no
// conversation on it never gets any.
func TestPendingCountIsZeroWithNoThreads(t *testing.T) {
	t.Parallel()

	var c domain.ReviewContext
	if got := c.PendingCount(); got != 0 {
		t.Errorf("PendingCount() = %d, want 0", got)
	}
}
