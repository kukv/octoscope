package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// crossRepoLister is what an operation that cannot name a single repository
// takes: unlike lister's operations, none of these are "the contents of one
// named repository".
type crossRepoLister interface {
	ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error)
	RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error)
	SearchItems(ctx context.Context, query string) ([]domain.WorkItem, error)
}

func (u *Usecase) ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error) {
	return u.crossRepo.ListWorkSection(ctx, s)
}

func (u *Usecase) RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error) {
	return u.crossRepo.RepoCounts(ctx, repos)
}
