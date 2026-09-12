package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed repo_name.graphql
var repoNameQuery string

// RepoName returns the repository's canonical "owner/name".
func (c *Client) RepoName(ctx context.Context, repo string) (string, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return "", err
	}
	out, err := c.Read(ctx, repoNameQuery, vars...)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Repository struct {
				NameWithOwner string `json:"nameWithOwner"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse repository name: %w", err)
	}
	return resp.Data.Repository.NameWithOwner, nil
}
