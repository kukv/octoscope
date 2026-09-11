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
	PageInfo pageInfo          `json:"pageInfo"`
	Nodes    []checkDetailNode `json:"nodes"`
}

type pageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

// checkDetailNode is the rollup context with the fields the checks view acts
// on. checkNode (graphql.go) stays as it is: the board selects fewer fields.
type checkDetailNode struct {
	checkNode
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
		out, err := c.read(ctx, c.dir, args...)
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
	out, err := c.read(ctx, c.dir, appendRepo(args, c.effectiveRepo(repo))...)
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

// RerunWorkflow starts a workflow run again. GraphQL has no mutation for
// this, which is why it goes through the subcommand.
func (c *Client) RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error {
	args := []string{"run", "rerun", strconv.FormatInt(runID, 10)}
	if scope == gh.RerunFailed {
		args = append(args, "--failed")
	}
	_, err := c.run(ctx, c.dir, appendRepo(args, c.effectiveRepo(repo))...)
	return err
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
