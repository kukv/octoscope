package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

type merger interface {
	PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error)
	MergePR(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error
	EnableAutoMerge(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error
	DisableAutoMerge(ctx context.Context, pr domain.PullRequestHandle) error
}

func (u *Usecase) PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error) {
	return u.merges.PRMergeContext(ctx, repo, number)
}

func (u *Usecase) MergePR(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error {
	return u.merges.MergePR(ctx, pr, method)
}

func (u *Usecase) EnableAutoMerge(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error {
	return u.merges.EnableAutoMerge(ctx, pr, method)
}

func (u *Usecase) DisableAutoMerge(ctx context.Context, pr domain.PullRequestHandle) error {
	return u.merges.DisableAutoMerge(ctx, pr)
}
