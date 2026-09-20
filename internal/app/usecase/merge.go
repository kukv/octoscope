package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

type merger interface {
	PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error)
	MergePR(pr domain.PullRequestHandle, method domain.MergeMethod) error
	EnableAutoMerge(pr domain.PullRequestHandle, method domain.MergeMethod) error
	DisableAutoMerge(pr domain.PullRequestHandle) error
}

func (u *Usecase) PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error) {
	return u.merges.PRMergeContext(ctx, repo, number)
}

func (u *Usecase) MergePR(pr domain.PullRequestHandle, method domain.MergeMethod) error {
	return u.merges.MergePR(pr, method)
}

func (u *Usecase) EnableAutoMerge(pr domain.PullRequestHandle, method domain.MergeMethod) error {
	return u.merges.EnableAutoMerge(pr, method)
}

func (u *Usecase) DisableAutoMerge(pr domain.PullRequestHandle) error {
	return u.merges.DisableAutoMerge(pr)
}
