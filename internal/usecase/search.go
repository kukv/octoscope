package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/config"
	"github.com/kukv/octoscope/internal/gh"
)

// SearchItems runs the Search tab's query.
func (u *Usecase) SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error) {
	return u.crossRepo.SearchItems(ctx, query)
}

// SavedQuery is one of the Search tab's saved queries: what the user called
// it, and the GitHub search it stands for. It crosses the boundary because
// internal/tui cannot see internal/config, and this package can see both.
type SavedQuery struct {
	Name  string
	Query string
}

// SaveQueries writes the Search tab's saved queries to the settings file.
func (u *Usecase) SaveQueries(queries []SavedQuery) error {
	saved := make([]config.SavedQuery, len(queries))
	for i, q := range queries {
		saved[i] = config.SavedQuery{Name: q.Name, Query: q.Query}
	}
	return u.queryStore.SaveQueries(saved)
}

// SavedQueriesFrom converts the settings file's saved queries into the form
// the UI takes, for cmd/octoscope's startup wiring.
func SavedQueriesFrom(qs []config.SavedQuery) []SavedQuery {
	out := make([]SavedQuery, len(qs))
	for i, q := range qs {
		out[i] = SavedQuery{Name: q.Name, Query: q.Query}
	}
	return out
}
