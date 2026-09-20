package usecase

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

func TestMergePRPassesTheMethodThrough(t *testing.T) {
	t.Parallel()

	f := &fakeSource{}
	u := &Usecase{merges: f}
	if err := u.MergePR("PR_1", domain.MergeRebase); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if f.mergedID != "PR_1" || f.mergedMethod != domain.MergeRebase {
		t.Errorf("merged (%q, %v), want (%q, %v)", f.mergedID, f.mergedMethod, "PR_1", domain.MergeRebase)
	}
}
