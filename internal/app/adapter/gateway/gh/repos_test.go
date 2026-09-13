package gh

import (
	"context"
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
)

// fakeRepoFinder answers SearchRepos and ListOwnRepos with whatever a test
// sets. Embedding the nil backend panics loudly if a test calls a method it
// did not mean to exercise.
type fakeRepoFinder struct {
	backend
	searchRepos  func(ctx context.Context, query string, limit int) ([]github.Repository, error)
	listOwnRepos func(ctx context.Context, owner string, limit int) ([]github.Repository, error)
}

func (f fakeRepoFinder) SearchRepos(ctx context.Context, query string, limit int) ([]github.Repository, error) {
	return f.searchRepos(ctx, query, limit)
}

func (f fakeRepoFinder) ListOwnRepos(ctx context.Context, owner string, limit int) ([]github.Repository, error) {
	return f.listOwnRepos(ctx, owner, limit)
}

// Every field of github.Repository is non-zero and distinct so that
// dropping any one of them from the conversion leaves it unguarded.
var wireRepo = github.Repository{Name: "kukv/octoscope", Stars: 42, Private: true}

var wantRepoCandidate = domain.RepoCandidate{Name: "kukv/octoscope", Stars: 42, Private: true}

func TestSearchReposTranslatesEveryField(t *testing.T) {
	t.Parallel()

	g := New(fakeRepoFinder{searchRepos: func(context.Context, string, int) ([]github.Repository, error) {
		return []github.Repository{wireRepo}, nil
	}})

	got, err := g.SearchRepos(context.Background(), "octoscope", 5)
	if err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	want := []domain.RepoCandidate{wantRepoCandidate}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SearchRepos() = %+v, want %+v", got, want)
	}
}

func TestListOwnReposTranslatesEveryField(t *testing.T) {
	t.Parallel()

	g := New(fakeRepoFinder{listOwnRepos: func(context.Context, string, int) ([]github.Repository, error) {
		return []github.Repository{wireRepo}, nil
	}})

	got, err := g.ListOwnRepos(context.Background(), "kukv", 100)
	if err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	want := []domain.RepoCandidate{wantRepoCandidate}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListOwnRepos() = %+v, want %+v", got, want)
	}
}
