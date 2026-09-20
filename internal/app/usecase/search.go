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

// queryStore is where the Search tab's saved queries survive a restart.
type queryStore interface {
	SaveQueries(queries []domain.SavedQuery) error
}

// SearchItems runs the Search tab's query.
func (u *Usecase) SearchItems(ctx context.Context, query string) ([]domain.WorkItem, error) {
	return u.search.SearchItems(ctx, query)
}

// SaveQueries writes the Search tab's saved queries to the store.
func (u *Usecase) SaveQueries(queries []domain.SavedQuery) error {
	return u.queryStore.SaveQueries(queries)
}
