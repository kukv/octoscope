package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

type fakeSource struct {
	err error

	viewer    string
	viewerErr error

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

	mergeContext        domain.MergeContext
	mergedID            domain.PullRequestHandle
	mergedMethod        domain.MergeMethod
	autoMergeID         domain.PullRequestHandle
	autoMergeMethod     domain.MergeMethod
	disabledAutoMergeID domain.PullRequestHandle
}

func (f *fakeSource) Viewer(context.Context) (string, error) { return f.viewer, f.viewerErr }

func (f *fakeSource) PRChecks(_ context.Context, repo string, number int) (domain.Checks, error) {
	f.checksRepo, f.checksNumber = repo, number
	return f.checks, f.err
}

func (f *fakeSource) JobLog(_ context.Context, repo string, jobID domain.JobHandle, failedOnly bool) ([]domain.LogLine, error) {
	f.logRepo, f.logJobID, f.logFailed = repo, jobID, failedOnly
	return f.logLines, f.err
}

func (f *fakeSource) RerunWorkflow(_ context.Context, repo string, runID domain.RunHandle, scope domain.RerunScope) error {
	f.rerunRepo, f.rerunRunID, f.rerunScope = repo, runID, scope
	return f.err
}

func (f *fakeSource) PRMergeContext(_ context.Context, _ string, _ int) (domain.MergeContext, error) {
	return f.mergeContext, f.err
}

func (f *fakeSource) MergePR(pr domain.PullRequestHandle, method domain.MergeMethod) error {
	f.mergedID, f.mergedMethod = pr, method
	return f.err
}

func (f *fakeSource) EnableAutoMerge(pr domain.PullRequestHandle, method domain.MergeMethod) error {
	f.autoMergeID, f.autoMergeMethod = pr, method
	return f.err
}

func (f *fakeSource) DisableAutoMerge(pr domain.PullRequestHandle) error {
	f.disabledAutoMergeID = pr
	return f.err
}
