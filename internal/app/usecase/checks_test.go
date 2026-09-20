package usecase

import (
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

func TestPRChecksReachesTheBackend(t *testing.T) {
	t.Parallel()

	f := &fakeSource{checks: domain.Checks{Total: 3}}
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

	f := &fakeSource{logLines: []domain.LogLine{{Text: "hi"}}}
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

	f := &fakeSource{}
	u := &Usecase{checks: f}

	if err := u.RerunWorkflow(t.Context(), "kukv/octoscope", "61", domain.RerunAll); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if f.rerunRepo != "kukv/octoscope" || f.rerunRunID != "61" || f.rerunScope != domain.RerunAll {
		t.Errorf("backend was asked to rerun %s run %q scope=%v, want kukv/octoscope run 61 scope=RerunAll",
			f.rerunRepo, f.rerunRunID, f.rerunScope)
	}
}
