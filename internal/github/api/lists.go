package api

import (
	"context"
	"encoding/json"
	"fmt"
)

// pageSize is REST's maximum for one page, and the ceiling the cli backend
// reads too. Asking for more is a 422.
const pageSize = 100

// Author is the login GitHub attributes something to.
type Author struct {
	Login string `json:"login"`
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
	var users []Author
	if err := json.Unmarshal(out, &users); err != nil {
		return nil, fmt.Errorf("parse assignees: %w", err)
	}
	logins := make([]string, len(users))
	for i, u := range users {
		logins[i] = u.Login
	}
	return logins, nil
}
