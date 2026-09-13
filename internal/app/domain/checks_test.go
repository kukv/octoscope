package domain

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

// The runner stamps most lines with an RFC3339 timestamp and a space, and the
// very first line of a log carries a byte order mark before it. Both belong to
// the transport, not to what the step printed, so neither reaches the screen.
func TestALogLineKeepsTheStampOutOfTheText(t *testing.T) {
	t.Parallel()

	line := NewLogLine("Run tests", "\ufeff2026-09-07T09:15:22.1234567Z ok  \tgithub.com/kukv/octoscope\t0.4s")

	if line.Step != "Run tests" {
		t.Errorf("Step = %q, want %q", line.Step, "Run tests")
	}
	want := time.Date(2026, 9, 7, 9, 15, 22, 123456700, time.UTC)
	if !line.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", line.Time, want)
	}
	if line.Text != "ok  \tgithub.com/kukv/octoscope\t0.4s" {
		t.Errorf("Text = %q", line.Text)
	}
}

// A step's output can wrap onto lines the runner did not stamp. Those keep
// their whole text: cutting at the first space would eat a word.
func TestAnUnstampedLineKeepsItsWholeText(t *testing.T) {
	t.Parallel()

	line := NewLogLine("Run tests", "    expected 3, got 4")

	if !line.Time.IsZero() {
		t.Errorf("Time = %v, want the zero time", line.Time)
	}
	if line.Text != "    expected 3, got 4" {
		t.Errorf("Text = %q", line.Text)
	}
}

// A line whose first word merely looks like it could be a stamp is not one.
// Cutting it off would drop a word the step actually printed.
func TestAFirstWordThatIsNotATimestampStays(t *testing.T) {
	t.Parallel()

	line := NewLogLine("Run tests", "2026-09-07 09:15:22 starting")

	if !line.Time.IsZero() {
		t.Errorf("Time = %v, want the zero time", line.Time)
	}
	if line.Text != "2026-09-07 09:15:22 starting" {
		t.Errorf("Text = %q", line.Text)
	}
}
