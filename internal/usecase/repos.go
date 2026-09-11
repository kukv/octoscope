package usecase

import (
	"context"
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
)

// seedLimit is how many repositories one owner contributes to the seeding
// list. gh repo list fetches 30 by default; an account or an organisation
// with more than that would lose the rest without a word.
const seedLimit = 100

// SaveRepositories writes the sidebar's list to the settings file.
func (u *Usecase) SaveRepositories(repos []string) error {
	return u.repoStore.SaveRepositories(repos)
}

// SearchRepos looks for repositories to offer while the add dialog is being
// typed into.
func (u *Usecase) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	return u.repos.SearchRepos(ctx, query, limit)
}

// SeedCandidates is what a first run has to offer: the user's own
// repositories and those of every organisation they belong to. A token
// without the organisation scope is common enough that losing the rest over
// it would be the wrong trade, so an organisation that cannot be read is
// passed over rather than returned.
func (u *Usecase) SeedCandidates(ctx context.Context) ([]gh.RepoCandidate, error) {
	own, err := u.repos.ListOwnRepos(ctx, "", seedLimit)
	if err != nil {
		return nil, fmt.Errorf("list own repos: %w", err)
	}
	seen := make(map[string]bool, len(own))
	var out []gh.RepoCandidate
	add := func(candidates []gh.RepoCandidate) {
		for _, c := range candidates {
			if seen[c.Name] {
				continue
			}
			seen[c.Name] = true
			out = append(out, c)
		}
	}
	add(own)
	orgs, err := u.repos.ListOrgs(ctx)
	if err != nil {
		return out, nil
	}
	for _, org := range orgs {
		repos, err := u.repos.ListOwnRepos(ctx, org, seedLimit)
		if err != nil {
			continue
		}
		add(repos)
	}
	return out, nil
}
