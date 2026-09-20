package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
)

// itemPath is where one pull request or issue lives. The two have separate
// endpoints for their own fields, and share the issues one for everything
// GitHub gives both: comments, labels, assignees.
func (c *Client) itemPath(kind, repo string, number int) (string, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("repos/%s/%s/%d", r, kind, number), nil
}

// comment posts one conversation comment. A pull request takes this path too:
// /pulls/{n}/comments is the review threads instead, and a comment sent there
// would land on a diff line.
func (c *Client) comment(ctx context.Context, repo string, number int, body string) error {
	path, err := c.itemPath("issues", repo, number)
	if err != nil {
		return err
	}
	_, err = c.write(ctx, http.MethodPost, path+"/comments",
		map[string]string{"body": body})
	return err
}

func (c *Client) AddPRComment(ctx context.Context, repo string, number int, body string) error {
	return c.comment(ctx, repo, number, body)
}

func (c *Client) AddIssueComment(ctx context.Context, repo string, number int, body string) error {
	return c.comment(ctx, repo, number, body)
}

// setState opens or closes one item. kind picks the endpoint: a pull request
// is closed through /pulls, the way gh pr close does it.
func (c *Client) setState(ctx context.Context, kind, repo string, number int, state string) error {
	path, err := c.itemPath(kind, repo, number)
	if err != nil {
		return err
	}
	_, err = c.write(ctx, http.MethodPatch, path,
		map[string]string{"state": state})
	return err
}

func (c *Client) ClosePR(ctx context.Context, repo string, number int) error {
	return c.setState(ctx, "pulls", repo, number, "closed")
}

func (c *Client) ReopenPR(ctx context.Context, repo string, number int) error {
	return c.setState(ctx, "pulls", repo, number, "open")
}

func (c *Client) CloseIssue(ctx context.Context, repo string, number int) error {
	return c.setState(ctx, "issues", repo, number, "closed")
}

func (c *Client) ReopenIssue(ctx context.Context, repo string, number int) error {
	return c.setState(ctx, "issues", repo, number, "open")
}

// editLabels adds and removes labels on one item. Additions travel together;
// a removal names its label in the path, one request each, which is the only
// shape GitHub offers.
func (c *Client) editLabels(ctx context.Context, repo string, number int, add, remove []string) error {
	path, err := c.itemPath("issues", repo, number)
	if err != nil {
		return err
	}
	if len(add) > 0 {
		if _, err := c.write(ctx, http.MethodPost, path+"/labels",
			map[string][]string{"labels": add}); err != nil {
			return err
		}
	}
	for _, name := range remove {
		// A label name is a path segment and GitHub's own have slashes in
		// them ("area/cli"), which would otherwise split the path instead of
		// naming one label.
		if _, err := c.write(ctx, http.MethodDelete,
			path+"/labels/"+url.PathEscape(name), nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) EditPRLabels(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editLabels(ctx, repo, number, add, remove)
}

func (c *Client) EditIssueLabels(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editLabels(ctx, repo, number, add, remove)
}

// editAssignees adds and removes assignees on one item. Both sides take a
// list in the body at the same path; the method is what separates them.
func (c *Client) editAssignees(ctx context.Context, repo string, number int, add, remove []string) error {
	path, err := c.itemPath("issues", repo, number)
	if err != nil {
		return err
	}
	if len(add) > 0 {
		if _, err := c.write(ctx, http.MethodPost, path+"/assignees",
			map[string][]string{"assignees": add}); err != nil {
			return err
		}
	}
	if len(remove) > 0 {
		if _, err := c.write(ctx, http.MethodDelete, path+"/assignees",
			map[string][]string{"assignees": remove}); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) EditPRAssignees(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editAssignees(ctx, repo, number, add, remove)
}

func (c *Client) EditIssueAssignees(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editAssignees(ctx, repo, number, add, remove)
}
