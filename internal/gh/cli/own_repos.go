package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
)

type ownRepoJSON struct {
	NameWithOwner string `json:"nameWithOwner"`
	IsPrivate     bool   `json:"isPrivate"`
}

// ListOwnRepos lists the repositories of owner, or of the authenticated user
// when owner is empty. It carries no star count: gh repo list offers none,
// and this list is read by name.
func (c *Client) ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error) {
	args := []string{"repo", "list"}
	if owner != "" {
		args = append(args, owner)
	}
	args = append(args, "--json", "nameWithOwner,isPrivate", "--limit", strconv.Itoa(limit))
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	var found []ownRepoJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo list: %w", err)
	}
	repos := make([]gh.RepoCandidate, len(found))
	for i, f := range found {
		repos[i] = gh.RepoCandidate{Name: f.NameWithOwner, Private: f.IsPrivate}
	}
	return repos, nil
}

// ListOrgs names the organisations the authenticated user belongs to. The
// --jq expression wraps the logins back into an array: without the brackets
// gh prints one bare word per line, which is not JSON.
func (c *Client) ListOrgs(ctx context.Context) ([]string, error) {
	out, err := c.read(ctx, c.dir, "api", "user/orgs", "--jq", "[.[].login]")
	if err != nil {
		return nil, err
	}
	var logins []string
	if err := json.Unmarshal(out, &logins); err != nil {
		return nil, fmt.Errorf("parse orgs: %w", err)
	}
	return logins, nil
}
