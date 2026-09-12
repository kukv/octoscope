package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/gh"
)

// SearchItems runs the Search tab's query.
func (u *Usecase) SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error) {
	return u.crossRepo.SearchItems(ctx, query)
}
