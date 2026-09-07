package cli

import (
	"context"
	"errors"
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

// rollupPage wraps contexts nodes in the shape the query selects them in.
func rollupPage(pageInfo, nodes string) string {
	return `{"data":{"repository":{"pullRequest":{"commits":{"nodes":[{"commit":` +
		`{"statusCheckRollup":{"contexts":{"pageInfo":` + pageInfo + `,"nodes":[` + nodes + `]}}}}]}}}}}`
}

func TestPRChecksWalksEveryPageOfContexts(t *testing.T) {
	t.Parallel()

	page1 := rollupPage(`{"hasNextPage":true,"endCursor":"CUR1"}`,
		`{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"SUCCESS"}`)
	page2 := rollupPage(`{"hasNextPage":false,"endCursor":"CUR2"}`,
		`{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE"}`)

	f := &fakeSeq{outs: []string{page1, page2}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	checks, err := c.PRChecks(t.Context(), "", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (the first page says there is another)", len(f.calls))
	}
	// Without the cursor the second request asks for page one again and the
	// loop never ends.
	if !slices.Contains(f.calls[1], "after=CUR1") {
		t.Errorf("second call = %v, want it to carry after=CUR1", f.calls[1])
	}
	if checks.Total != 2 {
		t.Errorf("Total = %d, want 2 (one from each page)", checks.Total)
	}
}

func TestPRChecksReadsTheIdsTheViewActsOn(t *testing.T) {
	t.Parallel()

	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: fileRun(t, "testdata/pr_checks.json")}
	checks, err := c.PRChecks(t.Context(), "", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	i := slices.IndexFunc(checks.Runs, func(r gh.CheckRun) bool { return r.Name == "lint" })
	if i < 0 {
		t.Fatalf("no check named lint in %v", checks.Runs)
	}
	got := checks.Runs[i]
	if got.Kind != gh.CheckKindRun {
		t.Errorf("Kind = %v, want CheckKindRun", got.Kind)
	}
	if got.JobID != 101635448466 {
		t.Errorf("JobID = %d, want 101635448466", got.JobID)
	}
	if got.RunID != 34087925535 {
		t.Errorf("RunID = %d, want 34087925535", got.RunID)
	}
	if got.Workflow != "CI" {
		t.Errorf("Workflow = %q, want %q", got.Workflow, "CI")
	}
	if got.Duration() == 0 {
		t.Error("Duration() = 0, want the time between startedAt and completedAt")
	}
}

func TestJobLogStripsTheJobNameAndTheByteOrderMark(t *testing.T) {
	t.Parallel()

	c := &Client{repo: "kukv/octoscope", run: fileRun(t, "testdata/job_log.txt")}
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

	c := &Client{repo: "kukv/octoscope", run: fileRun(t, "testdata/job_log_failed.txt")}
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

	var got []string
	c := &Client{repo: "kukv/octoscope", run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}}
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

	c := &Client{repo: "kukv/octoscope", run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, nil // gh exits 0 with no output when no step failed
	}}
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
	c := &Client{repo: "kukv/octoscope", run: func(context.Context, string, ...string) ([]byte, error) {
		return nil, wantErr
	}}
	_, err = c.JobLog(context.Background(), "", 101759970990, false)
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestRerunAsksForOnlyTheFailedJobsWhenScopedThatWay(t *testing.T) {
	t.Parallel()

	var got []string
	c := &Client{repo: "kukv/octoscope", run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}}
	if err := c.RerunWorkflow(context.Background(), "", 34087925535, gh.RerunFailed); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	want := []string{"run", "rerun", "34087925535", "--failed", "--repo", "kukv/octoscope"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestRerunOfTheWholeRunPassesNoFailedFlag(t *testing.T) {
	t.Parallel()

	var got []string
	c := &Client{repo: "kukv/octoscope", run: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return nil, nil
	}}
	if err := c.RerunWorkflow(context.Background(), "", 34087925535, gh.RerunAll); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if slices.Contains(got, "--failed") {
		t.Errorf("args = %v, want no --failed", got)
	}
}
