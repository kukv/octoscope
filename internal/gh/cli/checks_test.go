package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// fileRun returns a run function that answers every call with one recorded
// response.
func fileRun(t *testing.T, path string) runFunc {
	t.Helper()

	recorded, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return func(context.Context, string, ...string) ([]byte, error) {
		return recorded, nil
	}
}

func TestJobLogStripsTheJobNameAndTheByteOrderMark(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope")
	c.run = fileRun(t, "testdata/job_log.txt")
	lines, err := c.JobLog(context.Background(), "", 101635448466, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) == 0 {
		t.Fatal("no lines")
	}
	first := lines[0]
	if strings.HasPrefix(first.Text, "\ufeff") {
		t.Errorf("Text keeps the byte order mark: %q", first.Text)
	}
	if strings.Contains(first.Text, "\t") {
		t.Errorf("Text keeps a tab, so a field was not stripped: %q", first.Text)
	}
	if first.Step == "" {
		t.Error("Step is empty, want the step name the log line carries")
	}
	if first.Time.IsZero() {
		t.Error("Time is zero, want the timestamp the line starts with")
	}
	if !strings.HasPrefix(first.Text, "Current runner version") {
		t.Errorf("Text = %q, want the message with the timestamp taken off", first.Text)
	}
}

func TestALineWithoutATimestampKeepsItsText(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope")
	c.run = fileRun(t, "testdata/job_log_failed.txt")
	lines, err := c.JobLog(context.Background(), "", 88970766114, true)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	// The recording has continuation lines: the osv-scanner step's scan-args
	// wrap onto lines the runner did not stamp.
	i := slices.IndexFunc(lines, func(l gh.LogLine) bool { return l.Time.IsZero() })
	if i < 0 {
		t.Fatalf("no continuation line among %d lines; the recording has some", len(lines))
	}
	if lines[i].Text == "" {
		t.Error("a continuation line lost its text")
	}
}

func TestJobLogAsksForOnlyTheFailedStepsWhenToldTo(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}
	if _, err := c.JobLog(context.Background(), "", 42, true); err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if !slices.Contains(got, "--log-failed") {
		t.Errorf("args = %v, want --log-failed", got)
	}
	if slices.Contains(got, "--log") {
		t.Errorf("args = %v, want no --log alongside --log-failed", got)
	}
}

func TestAJobThatDidNotFailReturnsNoLines(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, nil // gh exits 0 with no output when no step failed
	}
	lines, err := c.JobLog(context.Background(), "", 42, true)
	if err != nil {
		t.Fatalf("JobLog: %v, want no error: an empty log is not a failure", err)
	}
	if len(lines) != 0 {
		t.Errorf("lines = %v, want none", lines)
	}
}

// A job still running has no log to give: gh exits 1 and explains why on
// stderr instead of printing anything. JobLog must hand that error back
// unchanged so the checks view can show it as-is.
func TestJobLogPassesAnInProgressJobsErrorThroughUnchanged(t *testing.T) {
	t.Parallel()

	stderr, err := os.ReadFile("testdata/job_log_in_progress.txt")
	if err != nil {
		t.Fatal(err)
	}
	wantErr := fmt.Errorf("gh %s: %s", "run", strings.TrimSpace(string(stderr)))
	c := New("", "kukv/octoscope")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, wantErr
	}
	_, err = c.JobLog(context.Background(), "", 101759970990, false)
	// The text is what reaches the view, so a wrap that keeps errors.Is
	// happy but changes what the user reads must still fail this test.
	if err == nil || err.Error() != wantErr.Error() {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

// gh run rerun takes the run id as a positional argument, not a flag, and
// --failed is what limits the rerun to the jobs that failed rather than
// starting the whole run again.
func TestRerunFailedNamesTheRunAndAsksOnlyForFailedJobs(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}
	if err := c.RerunWorkflow(context.Background(), "", 34087925535, gh.RerunFailed); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if len(got) < 3 || got[0] != "run" || got[1] != "rerun" || got[2] != "34087925535" {
		t.Errorf("args = %v, want it to start with run rerun 34087925535", got)
	}
	if !slices.Contains(got, "--failed") {
		t.Errorf("args = %v, want --failed", got)
	}
	if !slices.Contains(got, "kukv/octoscope") {
		t.Errorf("args = %v, want the repository named", got)
	}
}

func TestRerunOfTheWholeRunPassesNoFailedFlag(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}
	if err := c.RerunWorkflow(context.Background(), "", 34087925535, gh.RerunAll); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if slices.Contains(got, "--failed") {
		t.Errorf("args = %v, want no --failed", got)
	}
}
