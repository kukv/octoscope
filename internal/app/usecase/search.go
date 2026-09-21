package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// itemSearcher runs a query that names no repository: the Search tab's
// results can come from anywhere the viewer can see.
type itemSearcher interface {
	SearchItems(ctx context.Context, query string) ([]domain.WorkItem, error)
}

type queryStore interface {
	SaveQueries(queries []domain.SavedQuery) error
}

func (u *Usecase) SearchItems(ctx context.Context, query string) ([]domain.WorkItem, error) {
	return u.search.SearchItems(ctx, query)
}

func (u *Usecase) SaveQueries(queries []domain.SavedQuery) error {
	return u.queryStore.SaveQueries(queries)
}
