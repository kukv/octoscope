package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

type checksFetcher interface {
	PRChecks(ctx context.Context, repo string, number int) (domain.Checks, error)
	JobLog(ctx context.Context, repo string, job domain.JobHandle, failedOnly bool) ([]domain.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, run domain.RunHandle, scope domain.RerunScope) error
}

func (u *Usecase) PRChecks(ctx context.Context, repo string, number int) (domain.Checks, error) {
	return u.checks.PRChecks(ctx, repo, number)
}

func (u *Usecase) JobLog(ctx context.Context, repo string, job domain.JobHandle, failedOnly bool) ([]domain.LogLine, error) {
	return u.checks.JobLog(ctx, repo, job, failedOnly)
}

func (u *Usecase) RerunWorkflow(ctx context.Context, repo string, run domain.RunHandle, scope domain.RerunScope) error {
	return u.checks.RerunWorkflow(ctx, repo, run, scope)
}
