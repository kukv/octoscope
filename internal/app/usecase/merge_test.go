package usecase

import (
	"context"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// fakeMerges is the merge port: the context the popup draws, and the three
// ways it can act on a pull request.
type fakeMerges struct {
	err error

	mergeContext        domain.MergeContext
	mergedID            domain.PullRequestHandle
	mergedMethod        domain.MergeMethod
	autoMergeID         domain.PullRequestHandle
	autoMergeMethod     domain.MergeMethod
	disabledAutoMergeID domain.PullRequestHandle
}

func (f *fakeMerges) PRMergeContext(_ context.Context, _ string, _ int) (domain.MergeContext, error) {
	return f.mergeContext, f.err
}

func (f *fakeMerges) MergePR(pr domain.PullRequestHandle, method domain.MergeMethod) error {
	f.mergedID, f.mergedMethod = pr, method
	return f.err
}

func (f *fakeMerges) EnableAutoMerge(pr domain.PullRequestHandle, method domain.MergeMethod) error {
	f.autoMergeID, f.autoMergeMethod = pr, method
	return f.err
}

func (f *fakeMerges) DisableAutoMerge(pr domain.PullRequestHandle) error {
	f.disabledAutoMergeID = pr
	return f.err
}

func TestMergePRPassesTheMethodThrough(t *testing.T) {
	t.Parallel()

	f := &fakeMerges{}
	u := &Usecase{merges: f}
	if err := u.MergePR("PR_1", domain.MergeRebase); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if f.mergedID != "PR_1" || f.mergedMethod != domain.MergeRebase {
		t.Errorf("merged (%q, %v), want (%q, %v)", f.mergedID, f.mergedMethod, "PR_1", domain.MergeRebase)
	}
}
