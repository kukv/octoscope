package api

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kukv/octoscope/internal/gh"
)

// pageSize is REST's maximum for one page, and the ceiling the cli backend
// reads too. Asking for more is a 422.
const pageSize = 100

// labelJSON is one entry of the labels endpoint. The id is what puts the list
// back in the order gh shows: gh asks GraphQL for CREATED_AT ascending, while
// REST answers by name, and a label's id rises with its creation.
type labelJSON struct {
	Name  string `json:"name"`
	Color string `json:"color"`
	ID    int64  `json:"id"`
}

// ListLabels names the repository's labels, oldest first. Nothing between
// here and the picker reorders them.
func (c *Client) ListLabels(ctx context.Context, repo string) ([]gh.Label, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.read(ctx, fmt.Sprintf("repos/%s/labels?per_page=%d", r, pageSize), "")
	if err != nil {
		return nil, err
	}
	var found []labelJSON
	if err := json.Unmarshal(out, &found); err != nil {
		return nil, fmt.Errorf("parse labels: %w", err)
	}
	slices.SortFunc(found, func(a, b labelJSON) int {
		switch {
		case a.ID < b.ID:
			return -1
		case a.ID > b.ID:
			return 1
		default:
			return 0
		}
	})
	labels := make([]gh.Label, len(found))
	for i, f := range found {
		labels[i] = gh.Label{Name: f.Name, Color: f.Color}
	}
	return labels, nil
}

// ListAssignees returns the logins of users assignable on the repository.
// The request is not paged: a repository with more than a page of assignable
// users would give the picker a list nobody could pick from anyway, and the
// cli backend reads the same one page.
func (c *Client) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.read(ctx, fmt.Sprintf("repos/%s/assignees?per_page=%d", r, pageSize), "")
	if err != nil {
		return nil, err
	}
	var users []gh.Author
	if err := json.Unmarshal(out, &users); err != nil {
		return nil, fmt.Errorf("parse assignees: %w", err)
	}
	logins := make([]string, len(users))
	for i, u := range users {
		logins[i] = u.Login
	}
	return logins, nil
}
