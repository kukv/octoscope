package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
)

const searchRepoFields = "fullName,stargazersCount,isPrivate"

type searchRepoJSON struct {
	FullName        string `json:"fullName"`
	StargazersCount int    `json:"stargazersCount"`
	IsPrivate       bool   `json:"isPrivate"`
}

// SearchRepos looks for repositories matching query. The query goes after
// "--" so that a word the user typed starting with a dash reaches gh as a
// search term rather than as a flag.
func (c *Client) SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	args := []string{
		"search", "repos",
		"--json", searchRepoFields,
		"--limit", strconv.Itoa(limit),
		"--", query,
	}
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	var found []searchRepoJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse repo search: %w", err)
	}
	candidates := make([]gh.RepoCandidate, len(found))
	for i, f := range found {
		candidates[i] = gh.RepoCandidate{
			Name:    f.FullName,
			Stars:   f.StargazersCount,
			Private: f.IsPrivate,
		}
	}
	return candidates, nil
}
