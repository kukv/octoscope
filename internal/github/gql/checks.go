package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

//go:embed checks.graphql
var checksQuery string

type checksResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				Commits struct {
					Nodes []commitNode `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// contexts walks to the rollup of the head commit. Nothing below the commit
// is optional in the schema except the rollup itself, which is null on a
// commit no check ever ran against.
func (r checksResponse) contexts() (contextPage, bool) {
	nodes := r.Data.Repository.PullRequest.Commits.Nodes
	if len(nodes) == 0 || nodes[0].Commit.StatusCheckRollup == nil {
		return contextPage{}, false
	}
	return nodes[0].Commit.StatusCheckRollup.Contexts, true
}

type commitNode struct {
	Commit struct {
		StatusCheckRollup *statusCheckRollup `json:"statusCheckRollup"`
	} `json:"commit"`
}

type statusCheckRollup struct {
	Contexts contextPage `json:"contexts"`
}

type contextPage struct {
	PageInfo PageInfo   `json:"pageInfo"`
	Nodes    []CheckRun `json:"nodes"`
}

// PageInfo is a GraphQL connection's paging cursor.
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// CheckRun is the rollup context with the fields the checks view acts on.
// CheckContext (search.go) stays as it is: the board selects fewer fields.
type CheckRun struct {
	CheckContext
	DatabaseID  int64          `json:"databaseId"`
	DetailsURL  string         `json:"detailsUrl"`
	TargetURL   string         `json:"targetUrl"`
	StartedAt   time.Time      `json:"startedAt"`
	CompletedAt time.Time      `json:"completedAt"`
	CreatedAt   time.Time      `json:"createdAt"`
	CheckSuite  checkSuiteNode `json:"checkSuite"`
}

type checkSuiteNode struct {
	// WorkflowRun is null for a check run an App created through the Checks
	// API: it reports to GitHub without an Actions run behind it.
	WorkflowRun *WorkflowRun `json:"workflowRun"`
}

// WorkflowRun names the Actions workflow a check run belongs to.
type WorkflowRun struct {
	DatabaseID int64 `json:"databaseId"`
	RunNumber  int   `json:"runNumber"`
	Workflow   struct {
		Name string `json:"name"`
	} `json:"workflow"`
}

// PRChecks fetches every check on the pull request's head commit.
func (c *Client) PRChecks(ctx context.Context, repo string, number int) ([]CheckRun, error) {
	repoFields, err := c.repoVars(repo)
	if err != nil {
		return nil, err
	}
	var nodes []CheckRun
	cursor := ""
	for {
		vars := append(slices.Clone(repoFields), N("number", number))
		if cursor != "" {
			vars = append(vars, S("after", cursor))
		}
		out, err := c.Read(ctx, checksQuery, vars...)
		if err != nil {
			return nil, err
		}
		var resp checksResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, fmt.Errorf("parse checks: %w", err)
		}
		contexts, ok := resp.contexts()
		if !ok {
			return nil, nil
		}
		nodes = append(nodes, contexts.Nodes...)
		if !contexts.PageInfo.HasNextPage || contexts.PageInfo.EndCursor == "" {
			break
		}
		cursor = contexts.PageInfo.EndCursor
	}
	return nodes, nil
}
