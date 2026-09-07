package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
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
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								Contexts struct {
									PageInfo struct {
										HasNextPage bool   `json:"hasNextPage"`
										EndCursor   string `json:"endCursor"`
									} `json:"pageInfo"`
									Nodes []checkDetailNode `json:"nodes"`
								} `json:"contexts"`
							} `json:"statusCheckRollup"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// checkDetailNode is the rollup context with the fields the checks view acts
// on. checkNode (graphql.go) stays as it is: the board selects fewer fields.
type checkDetailNode struct {
	checkNode
	DatabaseID  int64     `json:"databaseId"`
	DetailsURL  string    `json:"detailsUrl"`
	TargetURL   string    `json:"targetUrl"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	CreatedAt   time.Time `json:"createdAt"`
	CheckSuite  struct {
		WorkflowRun *struct {
			DatabaseID int64 `json:"databaseId"`
			RunNumber  int   `json:"runNumber"`
			Workflow   struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
}

// PRChecks fetches every check on the pull request's head commit.
func (c *Client) PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error) {
	repoFields, err := repoArgs(c.effectiveRepo(repo))
	if err != nil {
		return gh.Checks{}, err
	}
	var nodes []checkDetailNode
	cursor := ""
	for {
		args := append([]string{"api", "graphql", "-f", "query=" + checksQuery}, repoFields...)
		args = append(args, "-F", "number="+strconv.Itoa(number))
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		out, err := c.run(ctx, c.dir, args...)
		if err != nil {
			return gh.Checks{}, err
		}
		var resp checksResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return gh.Checks{}, fmt.Errorf("parse checks: %w", err)
		}
		commits := resp.Data.Repository.PullRequest.Commits.Nodes
		if len(commits) == 0 || commits[0].Commit.StatusCheckRollup == nil {
			return gh.Checks{}, nil
		}
		contexts := commits[0].Commit.StatusCheckRollup.Contexts
		nodes = append(nodes, contexts.Nodes...)
		if !contexts.PageInfo.HasNextPage || contexts.PageInfo.EndCursor == "" {
			break
		}
		cursor = contexts.PageInfo.EndCursor
	}
	return detailRollup(nodes), nil
}

// detailRollup counts the contexts the same way rollup does, and keeps the
// ids the checks view needs alongside each one.
func detailRollup(nodes []checkDetailNode) gh.Checks {
	plain := make([]checkNode, 0, len(nodes))
	for _, n := range nodes {
		plain = append(plain, n.checkNode)
	}
	checks := rollup(plain)
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
