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
	PageInfo PageInfo          `json:"pageInfo"`
	Nodes    []checkDetailNode `json:"nodes"`
}

// PageInfo is a GraphQL connection's paging cursor.
type PageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// checkDetailNode is the rollup context with the fields the checks view acts
// on. CheckContext (search.go) stays as it is: the board selects fewer fields.
type checkDetailNode struct {
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
	WorkflowRun *workflowRunNode `json:"workflowRun"`
}

type workflowRunNode struct {
	DatabaseID int64 `json:"databaseId"`
	RunNumber  int   `json:"runNumber"`
	Workflow   struct {
		Name string `json:"name"`
	} `json:"workflow"`
}

// PRChecks fetches every check on the pull request's head commit.
func (c *Client) PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error) {
	repoFields, err := c.repoVars(repo)
	if err != nil {
		return gh.Checks{}, err
	}
	var nodes []checkDetailNode
	cursor := ""
	for {
		vars := append(slices.Clone(repoFields), N("number", number))
		if cursor != "" {
			vars = append(vars, S("after", cursor))
		}
		out, err := c.Read(ctx, checksQuery, vars...)
		if err != nil {
			return gh.Checks{}, err
		}
		var resp checksResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return gh.Checks{}, fmt.Errorf("parse checks: %w", err)
		}
		contexts, ok := resp.contexts()
		if !ok {
			return gh.Checks{}, nil
		}
		nodes = append(nodes, contexts.Nodes...)
		if !contexts.PageInfo.HasNextPage || contexts.PageInfo.EndCursor == "" {
			break
		}
		cursor = contexts.PageInfo.EndCursor
	}
	return detailRollup(nodes), nil
}

// detailRollup counts the contexts the same way RollupContexts does, and
// keeps the ids the checks view needs alongside each one.
func detailRollup(nodes []checkDetailNode) gh.Checks {
	plain := make([]CheckContext, 0, len(nodes))
	for _, n := range nodes {
		plain = append(plain, n.CheckContext)
	}
	checks := RollupContexts(plain)
	for i, n := range nodes {
		checks.Runs[i] = n.toRun(checks.Runs[i].State)
	}
	return checks
}

func (n checkDetailNode) toRun(state gh.CheckState) gh.CheckRun {
	run := gh.CheckRun{Name: n.name(), State: state, Kind: gh.CheckKindRun}
	if n.Typename == "StatusContext" {
		run.Kind = gh.CheckKindStatus
		run.URL = n.TargetURL
		run.StartedAt = n.CreatedAt
		return run
	}
	run.URL = n.DetailsURL
	run.JobID = n.DatabaseID
	run.StartedAt = n.StartedAt
	run.CompletedAt = n.CompletedAt
	if wr := n.CheckSuite.WorkflowRun; wr != nil {
		run.RunID = wr.DatabaseID
		run.RunNumber = wr.RunNumber
		run.Workflow = wr.Workflow.Name
	}
	return run
}
