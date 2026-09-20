package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// crossRepoLister is what the Work board takes: unlike lister's operations,
// neither of these is "the contents of one named repository".
type crossRepoLister interface {
	ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error)
	RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error)
}

func (u *Usecase) ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error) {
	return u.crossRepo.ListWorkSection(ctx, s)
}

func (u *Usecase) RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error) {
	return u.crossRepo.RepoCounts(ctx, repos)
}
