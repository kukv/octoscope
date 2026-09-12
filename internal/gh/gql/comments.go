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

//go:embed pr_comments.graphql
var prCommentsQuery string

//go:embed issue_comments.graphql
var issueCommentsQuery string

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
