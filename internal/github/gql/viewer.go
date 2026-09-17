package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed viewer.graphql
var viewerQuery string

// Viewer returns the login of the signed-in user. It takes no repository:
// who is signed in is a fact about the token, not about a repository, which
// is why this is the one read here that sends no variables.
func (c *Client) Viewer(ctx context.Context) (string, error) {
	out, err := c.Read(ctx, viewerQuery)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			Viewer struct {
				Login string `json:"login"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse viewer: %w", err)
	}
	return resp.Data.Viewer.Login, nil
}
