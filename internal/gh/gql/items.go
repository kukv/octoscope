package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed repo_prs.graphql
var repoPRsQuery string

//go:embed repo_issues.graphql
var repoIssuesQuery string

//go:embed pr.graphql
var prQuery string

//go:embed issue.graphql
var issueQuery string

//go:embed pr_comments.graphql
var prCommentsQuery string

//go:embed issue_comments.graphql
var issueCommentsQuery string

//go:embed repo_name.graphql
var repoNameQuery string

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

// commentPage is one page of a conversation. GitHub caps a connection at
// 100 nodes and answers with the oldest of them, so a thread longer than
// that is only whole once the pages after the first have been walked.
type commentPage struct {
	PageInfo PageInfo      `json:"pageInfo"`
	Nodes    []commentNode `json:"nodes"`
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
	Comments commentPage `json:"comments"`
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

type prCommentsResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Comments commentPage `json:"comments"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type issueCommentsResponse struct {
	Data struct {
		Repository struct {
			Issue struct {
				Comments commentPage `json:"comments"`
			} `json:"issue"`
		} `json:"repository"`
	} `json:"data"`
}

func decodePRComments(body []byte) (commentPage, error) {
	var resp prCommentsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return commentPage{}, fmt.Errorf("parse pull request comments: %w", err)
	}
	return resp.Data.Repository.PullRequest.Comments, nil
}

func decodeIssueComments(body []byte) (commentPage, error) {
	var resp issueCommentsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return commentPage{}, fmt.Errorf("parse issue comments: %w", err)
	}
	return resp.Data.Repository.Issue.Comments, nil
}

// restOfConversation appends the pages of comments that come after the one
// the single-item document already carries. doc asks for nothing but the
// comments connection, so a long thread does not re-fetch the whole item.
func (c *Client) restOfConversation(ctx context.Context, doc string, repoVars []Var, number int, first commentPage,
	decode func([]byte) (commentPage, error),
) ([]commentNode, error) {
	nodes := first.Nodes
	page := first
	for page.PageInfo.HasNextPage && page.PageInfo.EndCursor != "" {
		vars := append(slices.Clone(repoVars), N("number", number), S("after", page.PageInfo.EndCursor))
		out, err := c.Read(ctx, doc, vars...)
		if err != nil {
			return nil, err
		}
		if page, err = decode(out); err != nil {
			return nil, err
		}
		nodes = append(nodes, page.Nodes...)
	}
	return nodes, nil
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

// GetPR returns one pull request with its body and conversation.
func (c *Client) GetPR(ctx context.Context, repo string, number int) (gh.PR, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return gh.PR{}, err
	}
	out, err := c.Read(ctx, prQuery, append(vars, N("number", number))...)
	if err != nil {
		return gh.PR{}, err
	}
	var resp prResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.PR{}, fmt.Errorf("parse pull request: %w", err)
	}
	node := resp.Data.Repository.PullRequest
	node.Comments.Nodes, err = c.restOfConversation(ctx, prCommentsQuery, vars, number, node.Comments, decodePRComments)
	if err != nil {
		return gh.PR{}, err
	}
	return node.toPR(), nil
}

// GetIssue returns one issue with its body and conversation.
func (c *Client) GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error) {
	vars, err := c.repoVars(repo)
	if err != nil {
		return gh.Issue{}, err
	}
	out, err := c.Read(ctx, issueQuery, append(vars, N("number", number))...)
	if err != nil {
		return gh.Issue{}, err
	}
	var resp issueResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.Issue{}, fmt.Errorf("parse issue: %w", err)
	}
	node := resp.Data.Repository.Issue
	node.Comments.Nodes, err = c.restOfConversation(ctx, issueCommentsQuery, vars, number, node.Comments, decodeIssueComments)
	if err != nil {
		return gh.Issue{}, err
	}
	return node.toIssue(), nil
}

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
