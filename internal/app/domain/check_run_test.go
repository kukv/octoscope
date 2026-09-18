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

func TestOnlyACheckRunWithARunBehindItHasAWorkflow(t *testing.T) {
	t.Parallel()

	run := CheckRun{
		Name:        "build",
		Kind:        CheckKindRun,
		WorkflowRun: RunHandle("R_1"),
	}
	if !run.HasWorkflow() {
		t.Error("a check run that names a workflow run says it has none")
	}

	// A StatusContext is an external service reporting a state and a link.
	status := CheckRun{Name: "codecov", Kind: CheckKindStatus}
	if status.HasWorkflow() {
		t.Error("a StatusContext says it has a workflow")
	}

	// A check run an App created has a null checkSuite.workflowRun behind it,
	// which leaves WorkflowRun empty.
	appRun := CheckRun{Name: "dependabot", Kind: CheckKindRun}
	if appRun.HasWorkflow() {
		t.Error("a check run with no run id says it has a workflow")
	}

	// Both halves of the condition are load-bearing. GitHub does not report a
	// StatusContext with a run id behind it, so this case is unreachable from
	// the wire; it is here so that dropping the Kind test does not go
	// unnoticed.
	impossible := CheckRun{Name: "codecov", Kind: CheckKindStatus, WorkflowRun: RunHandle("R_1")}
	if impossible.HasWorkflow() {
		t.Error("a StatusContext with a run id says it has a workflow")
	}
}
