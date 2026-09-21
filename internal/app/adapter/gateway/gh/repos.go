package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

func (g *Gateway) SearchRepos(ctx context.Context, query string, limit int) ([]domain.RepoCandidate, error) {
	found, err := g.backend.SearchRepos(ctx, query, limit)
	if err != nil {
		return nil, wrap(err)
	}
	candidates := make([]domain.RepoCandidate, len(found))
	for i, f := range found {
		candidates[i] = toRepoCandidate(f)
	}
	return candidates, nil
}

// ListOwnRepos lists the repositories of owner, or of the authenticated
// user when owner is empty.
func (g *Gateway) ListOwnRepos(ctx context.Context, owner string, limit int) ([]domain.RepoCandidate, error) {
	found, err := g.backend.ListOwnRepos(ctx, owner, limit)
	if err != nil {
		return nil, wrap(err)
	}
	repos := make([]domain.RepoCandidate, len(found))
	for i, f := range found {
		repos[i] = toRepoCandidate(f)
	}
	return repos, nil
}

// ValidRepoName reports whether name is one this service could have. The
// shape is GitHub's -- "owner/name", no second slash -- so the rule lives
// here rather than in the domain: another service spells it differently
// (GitLab nests groups) and the view must not have to know which.
func (g *Gateway) ValidRepoName(name string) bool {
	_, _, ok := gql.SplitRepo(name)
	return ok
}

func toRepoCandidate(r github.Repository) domain.RepoCandidate {
	return domain.RepoCandidate{Name: r.Name, Stars: r.Stars, Private: r.Private}
}
