package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
)

//go:embed repo_prs.graphql
var repoPRsQuery string

//go:embed repo_issues.graphql
var repoIssuesQuery string

//go:embed pr.graphql
var prQuery string

//go:embed issue.graphql
var issueQuery string

// prNode is one pull request as the documents in this package select it.
// The list document leaves Body, Comments and Assignees unselected and they
// decode as zero values; the single-item document fills them.
type prNode struct {
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
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels struct {
		Nodes []domain.Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []domain.Author `json:"nodes"`
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

func (n prNode) toPR() domain.PR {
	return domain.PR{
		Number:    n.Number,
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     domain.ParseItemState(n.State),
		IsDraft:   n.IsDraft,
		UpdatedAt: n.UpdatedAt,
		Review:    domain.ParseReviewDecision(n.ReviewDecision),
		URL:       n.URL,
		Body:      n.Body,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    n.Labels.Nodes,
		Assignees: n.Assignees.Nodes,
		Checks:    n.checks(),
		Head:      n.HeadRefName,
		Base:      n.BaseRefName,
		Additions: n.Additions,
		Deletions: n.Deletions,
	}
}

// checks reads the roll-up off the last commit, which is where GitHub hangs
// it: a pull request has no roll-up of its own.
func (n prNode) checks() domain.Checks {
	var nodes []CheckContext
	for _, commit := range n.Commits.Nodes {
		if rollup := commit.Commit.StatusCheckRollup; rollup != nil {
			nodes = append(nodes, rollup.Contexts.Nodes...)
		}
	}
	return RollupContexts(nodes)
}

type issueNode struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	State     string    `json:"state"`
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updatedAt"`
	Body      string    `json:"body"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	Labels struct {
		Nodes []domain.Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []domain.Author `json:"nodes"`
	} `json:"assignees"`
	Comments commentPage `json:"comments"`
}

func (n issueNode) toIssue() domain.Issue {
	return domain.Issue{
		Number:    n.Number,
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     domain.ParseItemState(n.State),
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
		Body:      n.Body,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    n.Labels.Nodes,
		Assignees: n.Assignees.Nodes,
	}
}

type prResponse struct {
	Data struct {
		Repository struct {
			PullRequest prNode `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type issueResponse struct {
	Data struct {
		Repository struct {
			Issue issueNode `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

type prListResponse struct {
	Data struct {
		Repository struct {
			PullRequests struct {
				Nodes []prNode `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
}

type issueListResponse struct {
	Data struct {
		Repository struct {
			Issues struct {
				Nodes []issueNode `json:"nodes"`
			} `json:"issues"`
		} `json:"repository"`
	} `json:"data"`
}

// ListPRs returns the open pull requests of one repository. An empty repo
// means "wherever we are", which only a transport that can answer that
// accepts.
func (c *Client) ListPRs(ctx context.Context, repo string) ([]domain.PR, error) {
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
	nodes := resp.Data.Repository.PullRequests.Nodes
	prs := make([]domain.PR, len(nodes))
	for i, n := range nodes {
		prs[i] = n.toPR()
	}
	return prs, nil
}

// ListIssues returns the open issues of one repository.
func (c *Client) ListIssues(ctx context.Context, repo string) ([]domain.Issue, error) {
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
	nodes := resp.Data.Repository.Issues.Nodes
	issues := make([]domain.Issue, len(nodes))
	for i, n := range nodes {
		issues[i] = n.toIssue()
	}
	return issues, nil
}

// GetPR returns one pull request with its body and conversation.
func (c *Client) GetPR(ctx context.Context, repo string, number int) (domain.PR, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return domain.PR{}, err
	}
	out, err := c.Read(ctx, prQuery, append(vars, N("number", number))...)
	if err != nil {
		return domain.PR{}, err
	}
	var resp prResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return domain.PR{}, fmt.Errorf("parse pull request: %w", err)
	}
	node := resp.Data.Repository.PullRequest
	node.Comments.Nodes, err = c.restOfConversation(ctx, prCommentsQuery, vars, number, node.Comments, decodePRComments)
	if err != nil {
		return domain.PR{}, err
	}
	return node.toPR(), nil
}

// GetIssue returns one issue with its body and conversation.
func (c *Client) GetIssue(ctx context.Context, repo string, number int) (domain.Issue, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return domain.Issue{}, err
	}
	out, err := c.Read(ctx, issueQuery, append(vars, N("number", number))...)
	if err != nil {
		return domain.Issue{}, err
	}
	var resp issueResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return domain.Issue{}, fmt.Errorf("parse issue: %w", err)
	}
	node := resp.Data.Repository.Issue
	node.Comments.Nodes, err = c.restOfConversation(ctx, issueCommentsQuery, vars, number, node.Comments, decodeIssueComments)
	if err != nil {
		return domain.Issue{}, err
	}
	return node.toIssue(), nil
}
