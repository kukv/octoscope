package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/kukv/octoscope/internal/gh"
)

// repoJSON is one repository as REST spells it, in the two shapes that carry
// it: the search results and the listings.
type repoJSON struct {
	FullName string `json:"full_name"`
	Stars    int    `json:"stargazers_count"`
	Private  bool   `json:"private"`
}

func (r repoJSON) toDomain() gh.RepoCandidate {
	return gh.RepoCandidate{Name: r.FullName, Stars: r.Stars, Private: r.Private}
}

// page is what REST will actually answer with. Asking for more than a page is
// a 422, so a caller's larger limit is cut down rather than sent.
func page(limit int) int {
	if limit > pageSize || limit <= 0 {
		return pageSize
	}
	return limit
}

// SearchRepos looks for repositories matching query.
func (c *Client) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	path := fmt.Sprintf("search/repositories?q=%s&per_page=%d",
		url.QueryEscape(query), page(limit))
	out, err := c.read(ctx, path, "")
	if err != nil {
		return nil, err
	}
	var found struct {
		Items []repoJSON `json:"items"`
	}
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo search: %w", err)
	}
	candidates := make([]gh.RepoCandidate, len(found.Items))
	for i, f := range found.Items {
		candidates[i] = f.toDomain()
	}
	return candidates, nil
}

// ListOwnRepos lists the repositories of owner, or of the authenticated user
// when owner is empty, most recently pushed first -- the order gh repo list
// shows them in.
//
// A named owner is read as an organisation: the only caller passes either an
// empty string or one of ListOrgs' answers. /users/{owner}/repos would answer
// for a person, but shows only public repositories.
func (c *Client) ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error) {
	path := fmt.Sprintf("user/repos?affiliation=owner&sort=pushed&direction=desc&per_page=%d", page(limit))
	if owner != "" {
		path = fmt.Sprintf("orgs/%s/repos?sort=pushed&direction=desc&per_page=%d",
			url.PathEscape(owner), page(limit))
	}
	out, err := c.read(ctx, path, "")
	if err != nil {
		return nil, err
	}
	var found []repoJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo list: %w", err)
	}
	repos := make([]gh.RepoCandidate, len(found))
	for i, f := range found {
		// The listings carry a star count too, but the seeding dialog reads
		// these by name; keeping it costs nothing and the field is there.
		repos[i] = f.toDomain()
	}
	return repos, nil
}

// ListOrgs names the organisations the authenticated user belongs to.
func (c *Client) ListOrgs(ctx context.Context) ([]string, error) {
	out, err := c.read(ctx, "user/orgs", "")
	if err != nil {
		return nil, err
	}
	var found []struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse orgs: %w", err)
	}
	logins := make([]string, len(found))
	for i, f := range found {
		logins[i] = f.Login
	}
	return logins, nil
}
