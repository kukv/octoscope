package gh

import (
	"testing"
	"time"
)

func TestDurationIsTheTimeBetweenStartAndFinish(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 9, 7, 5, 44, 0, 0, time.UTC)
	c := CheckRun{StartedAt: start, CompletedAt: start.Add(2*time.Minute + 14*time.Second)}
	if got, want := c.Duration(), 2*time.Minute+14*time.Second; got != want {
		t.Errorf("Duration() = %v, want %v", got, want)
	}
}

func TestARunningCheckHasNoDuration(t *testing.T) {
	t.Parallel()

	c := CheckRun{StartedAt: time.Date(2026, 9, 7, 5, 44, 0, 0, time.UTC)}
	if got := c.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0 while the check is still running", got)
	}
}

func TestACheckWithNoStartTimeHasNoDuration(t *testing.T) {
	t.Parallel()

	c := CheckRun{CompletedAt: time.Date(2026, 9, 7, 5, 44, 0, 0, time.UTC)}
	if got := c.Duration(); got != 0 {
		t.Errorf("Duration() = %v, want 0", got)
	}
}
