package usecase

import (
	"context"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// fakeChecks is the checks port: what the Checks pane asks for, and the one
// thing it asks the GitHub layer to do.
type fakeChecks struct {
	err error

	checks       domain.Checks
	checksRepo   string
	checksNumber int

	logLines  []domain.LogLine
	logRepo   string
	logJobID  domain.JobHandle
	logFailed bool

	rerunRepo  string
	rerunRunID domain.RunHandle
	rerunScope domain.RerunScope
}

func (f *fakeChecks) PRChecks(_ context.Context, repo string, number int) (domain.Checks, error) {
	f.checksRepo, f.checksNumber = repo, number
	return f.checks, f.err
}

func (f *fakeChecks) JobLog(_ context.Context, repo string, jobID domain.JobHandle, failedOnly bool) ([]domain.LogLine, error) {
	f.logRepo, f.logJobID, f.logFailed = repo, jobID, failedOnly
	return f.logLines, f.err
}

func (f *fakeChecks) RerunWorkflow(_ context.Context, repo string, runID domain.RunHandle, scope domain.RerunScope) error {
	f.rerunRepo, f.rerunRunID, f.rerunScope = repo, runID, scope
	return f.err
}

func TestPRChecksReachesTheBackend(t *testing.T) {
	t.Parallel()

	f := &fakeChecks{checks: domain.Checks{Total: 3}}
	u := &Usecase{checks: f}

	got, err := u.PRChecks(t.Context(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	if got.Total != 3 {
		t.Errorf("Total = %d, want 3", got.Total)
	}
	if f.checksRepo != "kukv/octoscope" || f.checksNumber != 61 {
		t.Errorf("backend was asked for %s#%d, want kukv/octoscope#61", f.checksRepo, f.checksNumber)
	}
}

func TestJobLogReachesTheBackend(t *testing.T) {
	t.Parallel()

	f := &fakeChecks{logLines: []domain.LogLine{{Text: "hi"}}}
	u := &Usecase{checks: f}

	got, err := u.JobLog(t.Context(), "kukv/octoscope", "61", true)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if !slices.Equal(got, f.logLines) {
		t.Errorf("JobLog = %+v, want %+v", got, f.logLines)
	}
	if f.logRepo != "kukv/octoscope" || f.logJobID != "61" || !f.logFailed {
		t.Errorf("backend was asked for %s job %q failedOnly=%v, want kukv/octoscope job 61 failedOnly=true",
			f.logRepo, f.logJobID, f.logFailed)
	}
}

func TestRerunWorkflowReachesTheBackend(t *testing.T) {
	t.Parallel()

	f := &fakeChecks{}
	u := &Usecase{checks: f}

	if err := u.RerunWorkflow(t.Context(), "kukv/octoscope", "61", domain.RerunAll); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if f.rerunRepo != "kukv/octoscope" || f.rerunRunID != "61" || f.rerunScope != domain.RerunAll {
		t.Errorf("backend was asked to rerun %s run %q scope=%v, want kukv/octoscope run 61 scope=RerunAll",
			f.rerunRepo, f.rerunRunID, f.rerunScope)
	}
}
