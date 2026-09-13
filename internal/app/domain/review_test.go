package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

func TestReviewTargetKeepsTheTwoHandlesApart(t *testing.T) {
	t.Parallel()
	tgt := domain.ReviewTarget{
		PullRequest: domain.PullRequestHandle("PR_1"),
		Pending:     domain.ReviewHandle("PRR_1"),
	}
	if string(tgt.PullRequest) == string(tgt.Pending) {
		t.Fatal("the two handles are the same value")
	}
}
