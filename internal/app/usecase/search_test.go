package usecase

import (
	"context"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// fakeSearcher records the query SearchItems was asked to run.
type fakeSearcher struct {
	searchQuery string
	err         error
}

func (f *fakeSearcher) SearchItems(_ context.Context, query string) ([]domain.WorkItem, error) {
	f.searchQuery = query
	return nil, f.err
}

func TestSearchItemsReachesTheGitHubLayer(t *testing.T) {
	t.Parallel()

	f := &fakeSearcher{}
	u := &Usecase{search: f}
	if _, err := u.SearchItems(t.Context(), "is:open is:pr"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if f.searchQuery != "is:open is:pr" {
		t.Errorf("the query reached the layer as %q", f.searchQuery)
	}
}

type fakeQueryStore struct {
	saved []domain.SavedQuery
	err   error
}

func (f *fakeQueryStore) SaveQueries(queries []domain.SavedQuery) error {
	f.saved = queries
	return f.err
}

func TestSaveQueriesReachesTheStore(t *testing.T) {
	t.Parallel()

	store := &fakeQueryStore{}
	u := &Usecase{queryStore: store}
	want := []domain.SavedQuery{{Name: "mine", Query: "is:open author:@me"}}
	if err := u.SaveQueries(want); err != nil {
		t.Fatalf("SaveQueries: %v", err)
	}
	if !slices.Equal(store.saved, want) {
		t.Errorf("store holds %+v, want %+v", store.saved, want)
	}
}
