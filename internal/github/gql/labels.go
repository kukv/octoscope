package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed labels.graphql
var labelsQuery string

// ListLabels names the repository's oldest hundred labels, oldest first.
func (c *Client) ListLabels(ctx context.Context, repo string) ([]Label, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.Read(ctx, labelsQuery, vars...)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			Repository struct {
				Labels struct {
					Nodes []Label `json:"nodes"`
				} `json:"labels"`
			} `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse labels: %w", err)
	}
	return resp.Data.Repository.Labels.Nodes, nil
}
