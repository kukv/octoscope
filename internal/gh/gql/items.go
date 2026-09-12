package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed repo_prs.graphql
var repoPRsQuery string

//go:embed repo_issues.graphql
var repoIssuesQuery string

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
		Nodes []gh.Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []gh.Author `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		Nodes []commentNode `json:"nodes"`
	} `json:"comments"`
	Commits struct {
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

// commentNode is one comment. GraphQL nests the author under an object,
// which gh.Comment already spells the same way.
type commentNode struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

func toComments(in []commentNode) []gh.Comment {
	if len(in) == 0 {
		return nil
	}
	out := make([]gh.Comment, len(in))
	for i, c := range in {
		out[i] = gh.Comment{
			Author:    gh.Author{Login: c.Author.Login},
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
		}
	}
	return out
}

func (n prNode) toPR() gh.PR {
	return gh.PR{
		Number:    n.Number,
		Title:     n.Title,
		Author:    gh.Author{Login: n.Author.Login},
		State:     gh.ParseItemState(n.State),
		IsDraft:   n.IsDraft,
		UpdatedAt: n.UpdatedAt,
		Review:    gh.ParseReviewDecision(n.ReviewDecision),
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
func (n prNode) checks() gh.Checks {
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
		Nodes []gh.Label `json:"nodes"`
	} `json:"labels"`
	Assignees struct {
		Nodes []gh.Author `json:"nodes"`
	} `json:"assignees"`
	Comments struct {
		Nodes []commentNode `json:"nodes"`
	} `json:"comments"`
}

func (n issueNode) toIssue() gh.Issue {
	return gh.Issue{
		Number:    n.Number,
		Title:     n.Title,
		Author:    gh.Author{Login: n.Author.Login},
		State:     gh.ParseItemState(n.State),
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
		Body:      n.Body,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    n.Labels.Nodes,
		Assignees: n.Assignees.Nodes,
	}
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
func (c *Client) ListPRs(ctx context.Context, repo string) ([]gh.PR, error) {
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
	prs := make([]gh.PR, len(nodes))
	for i, n := range nodes {
		prs[i] = n.toPR()
	}
	return prs, nil
}

// ListIssues returns the open issues of one repository.
func (c *Client) ListIssues(ctx context.Context, repo string) ([]gh.Issue, error) {
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
	issues := make([]gh.Issue, len(nodes))
	for i, n := range nodes {
		issues[i] = n.toIssue()
	}
	return issues, nil
}
