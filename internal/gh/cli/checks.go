package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

// JobLog reads one job's log. With failedOnly it asks for the failed steps
// alone: that is what someone opening a red check came to read, and a whole
// job's log has been measured at nine times the length.
//
// gh exits zero with no output at all when a successful job is asked for its
// failed steps, so an empty result is an answer, not an error.
func (c *Client) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error) {
	args := []string{"run", "view", "--job", strconv.FormatInt(jobID, 10)}
	if failedOnly {
		args = append(args, "--log-failed")
	} else {
		args = append(args, "--log")
	}
	out, err := c.run(ctx, c.dir, appendRepo(args, c.effectiveRepo(repo))...)
	if err != nil {
		return nil, err
	}
	return parseJobLog(string(out)), nil
}

// parseJobLog reads the format gh prints: three tab-separated fields, the
// job's name, the step's name, and the message, whose first word is an
// RFC3339 timestamp on every line the runner stamped. The message of the
// very first line starts with a byte order mark.
func parseJobLog(out string) []gh.LogLine {
	var lines []gh.LogLine
	for _, raw := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if raw == "" {
			continue
		}
		fields := strings.SplitN(raw, "\t", 3)
		if len(fields) < 3 {
			lines = append(lines, gh.LogLine{Text: fields[len(fields)-1]})
			continue
		}
		// The mark sits at the start of the message, not of the line: the
		// job and the step name come before it.
		message := strings.TrimPrefix(fields[2], "\ufeff")
		line := gh.LogLine{Step: fields[1], Text: message}
		if stamp, rest, ok := strings.Cut(message, " "); ok {
			if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
				line.Time, line.Text = at, rest
			}
		}
		lines = append(lines, line)
	}
	return lines
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
