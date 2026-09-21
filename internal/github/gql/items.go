package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed repo_prs.graphql
var repoPRsQuery string

//go:embed repo_issues.graphql
var repoIssuesQuery string

//go:embed pr.graphql
var prQuery string

//go:embed issue.graphql
var issueQuery string

// Label is a label as GitHub's documents select it.
type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Author is the login GitHub attributes something to.
type Author struct {
	Login string `json:"login"`
}

// PullRequest is one pull request as the documents in this package select
// it. The list document leaves Body, Comments and Assignees unselected and
// they decode as zero values; the single-item document fills them. BodyText
// goes the other way: only the list selects it.
type PullRequest struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	UpdatedAt      time.Time `json:"updatedAt"`
	ReviewDecision string    `json:"reviewDecision"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	Body           string    `json:"body"`
	BodyText       string    `json:"bodyText"`
	Author         Author    `json:"author"`
	Labels         struct {
		Nodes []Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []Author `json:"nodes"`
	} `json:"assignees"`
	Comments commentPage `json:"comments"`
	Commits  struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []CheckContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// StatusCheckContexts reads the roll-up off the last commit, which is where
// GitHub hangs it: a pull request has no roll-up of its own.
func (n PullRequest) StatusCheckContexts() []CheckContext {
	var nodes []CheckContext
	for _, commit := range n.Commits.Nodes {
		if rollup := commit.Commit.StatusCheckRollup; rollup != nil {
			nodes = append(nodes, rollup.Contexts.Nodes...)
		}
	}
	return nodes
}

// Issue is one issue as the documents in this package select it. Body and
// BodyText come from different documents, as PullRequest's do.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
	Body      string    `json:"body"`
	BodyText  string    `json:"bodyText"`
	Author    Author    `json:"author"`
	Labels    struct {
		Nodes []Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []Author `json:"nodes"`
	} `json:"assignees"`
	Comments commentPage `json:"comments"`
}

type prResponse struct {
	Data struct {
		Repository struct {
			PullRequest PullRequest `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type issueResponse struct {
	Data struct {
		Repository struct {
			Issue Issue `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

type prListResponse struct {
	Data struct {
		Repository struct {
			PullRequests struct {
				Nodes []PullRequest `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
}

type issueListResponse struct {
	Data struct {
		Repository struct {
			Issues struct {
				Nodes []Issue `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	} `json:"data"`
}

// ListPRs returns the open pull requests of one repository. An empty repo
// means "wherever we are", which only a transport that can answer that
// accepts.
func (c *Client) ListPRs(ctx context.Context, repo string) ([]PullRequest, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.Read(ctx, repoPRsQuery, vars...)
	if err != nil {
		return nil, err
	}
	var resp prListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse pull request list: %w", err)
	}
	return resp.Data.Repository.PullRequests.Nodes, nil
}

func (c *Client) ListIssues(ctx context.Context, repo string) ([]Issue, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return nil, err
	}
	out, err := c.Read(ctx, repoIssuesQuery, vars...)
	if err != nil {
		return nil, err
	}
	var resp issueListResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse issue list: %w", err)
	}
	return resp.Data.Repository.Issues.Nodes, nil
}

func (c *Client) GetPR(ctx context.Context, repo string, number int) (PullRequest, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return PullRequest{}, err
	}
	out, err := c.Read(ctx, prQuery, append(vars, N("number", number))...)
	if err != nil {
		return PullRequest{}, err
	}
	var resp prResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return PullRequest{}, fmt.Errorf("parse pull request: %w", err)
	}
	node := resp.Data.Repository.PullRequest
	node.Comments.Nodes, err = c.restOfConversation(ctx, prCommentsQuery, vars, number, node.Comments, decodePRComments)
	if err != nil {
		return PullRequest{}, err
	}
	return node, nil
}

func (c *Client) GetIssue(ctx context.Context, repo string, number int) (Issue, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return Issue{}, err
	}
	out, err := c.Read(ctx, issueQuery, append(vars, N("number", number))...)
	if err != nil {
		return Issue{}, err
	}
	var resp issueResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return Issue{}, fmt.Errorf("parse issue: %w", err)
	}
	node := resp.Data.Repository.Issue
	node.Comments.Nodes, err = c.restOfConversation(ctx, issueCommentsQuery, vars, number, node.Comments, decodeIssueComments)
	if err != nil {
		return Issue{}, err
	}
	return node, nil
}
