package gh

import (
	"context"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// PRChecks fetches every check on the pull request's head commit and rolls
// them up into the counts the checks view and the progress bar both need.
func (g *Gateway) PRChecks(ctx context.Context, repo string, number int) (domain.Checks, error) {
	runs, err := g.backend.PRChecks(ctx, repo, number)
	if err != nil {
		return domain.Checks{}, wrap(err)
	}
	return toChecks(runs), nil
}

// JobLog reads one job's log. The handle is the Actions database id the
// gateway handed out; anything else is a caller bug, not a service failure.
func (g *Gateway) JobLog(ctx context.Context, repo string, job domain.JobHandle, failedOnly bool) ([]domain.LogLine, error) {
	id, err := strconv.ParseInt(string(job), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("job handle %q: %w", job, err)
	}
	lines, err := g.backend.JobLog(ctx, repo, id, failedOnly)
	if err != nil {
		return nil, wrap(err)
	}
	out := make([]domain.LogLine, len(lines))
	for i, l := range lines {
		out[i] = toLogLine(l)
	}
	return out, nil
}

// RerunWorkflow starts a workflow run again. The handle is the Actions
// database id the gateway handed out; anything else is a caller bug, not a
// service failure.
func (g *Gateway) RerunWorkflow(ctx context.Context, repo string, run domain.RunHandle, scope domain.RerunScope) error {
	id, err := strconv.ParseInt(string(run), 10, 64)
	if err != nil {
		return fmt.Errorf("run handle %q: %w", run, err)
	}
	return wrap(g.backend.RerunWorkflow(ctx, repo, id, fromRerunScope(scope)))
}

// toChecks counts every check run once: each one increments Total and
// exactly one of Passed, Failed, or Running, so Passed+Failed+Running
// always equals Total.
func toChecks(runs []gql.CheckRun) domain.Checks {
	var c domain.Checks
	for _, n := range runs {
		run := toCheckRun(n)
		c.Total++
		c.Runs = append(c.Runs, run)
		switch run.State {
		case domain.CheckSuccess:
			c.Passed++
		case domain.CheckFailure:
			c.Failed++
		default:
			c.Running++
		}
	}
	switch {
	case c.Total == 0:
		c.State = domain.CheckNone
	case c.Failed > 0:
		c.State = domain.CheckFailure
	case c.Running > 0:
		c.State = domain.CheckRunning
	default:
		c.State = domain.CheckSuccess
	}
	return c
}

// toChecksFromContexts rolls up the check contexts a search or item
// document embeds under a commit -- a different query from the
// per-pull-request one toChecks reads, so it counts the same way but starts
// from the plainer gql.CheckContext shape. Only Name, State and Kind are
// filled on each run: the rollup carries nothing else (domain.CheckRun's own
// comment says why).
func toChecksFromContexts(nodes []gql.CheckContext) domain.Checks {
	var c domain.Checks
	for _, n := range nodes {
		kind := domain.CheckKindRun
		if n.Typename == "StatusContext" {
			kind = domain.CheckKindStatus
		}
		run := domain.CheckRun{Name: checkContextName(n), State: toCheckState(n), Kind: kind}
		c.Total++
		c.Runs = append(c.Runs, run)
		switch run.State {
		case domain.CheckSuccess:
			c.Passed++
		case domain.CheckFailure:
			c.Failed++
		default:
			c.Running++
		}
	}
	switch {
	case c.Total == 0:
		c.State = domain.CheckNone
	case c.Failed > 0:
		c.State = domain.CheckFailure
	case c.Running > 0:
		c.State = domain.CheckRunning
	default:
		c.State = domain.CheckSuccess
	}
	return c
}

// toCheckRun converts one rollup context. A StatusContext and a CheckRun
// report themselves through different fields, so the two shapes are read
// separately.
func toCheckRun(n gql.CheckRun) domain.CheckRun {
	run := domain.CheckRun{Name: checkRunName(n), State: toCheckState(n.CheckContext), Kind: domain.CheckKindRun}
	if n.Typename == "StatusContext" {
		run.Kind = domain.CheckKindStatus
		run.URL = n.TargetURL
		run.StartedAt = n.CreatedAt
		return run
	}
	run.URL = n.DetailsURL
	if n.DatabaseID != 0 {
		run.Job = domain.JobHandle(strconv.FormatInt(n.DatabaseID, 10))
	}
	run.StartedAt = n.StartedAt
	run.CompletedAt = n.CompletedAt
	if wr := n.CheckSuite.WorkflowRun; wr != nil {
		if wr.DatabaseID != 0 {
			run.WorkflowRun = domain.RunHandle(strconv.FormatInt(wr.DatabaseID, 10))
		}
		run.RunNumber = wr.RunNumber
		run.Workflow = wr.Workflow.Name
	}
	return run
}

// checkRunName is what the check calls itself. The two shapes spell the
// field differently, so the choice cannot be made by the JSON tags alone.
func checkRunName(n gql.CheckRun) string {
	return checkContextName(n.CheckContext)
}

// checkContextName is what one rollup context calls itself: a CheckRun
// spells its name "name", a StatusContext calls it "context".
func checkContextName(n gql.CheckContext) string {
	if n.Typename == "StatusContext" {
		return n.Context
	}
	return n.Name
}

// toCheckState reads one context of the rollup. CheckRun reports status and
// conclusion; the older StatusContext reports a single state, so the two
// shapes have to be read differently.
func toCheckState(n gql.CheckContext) domain.CheckState {
	if n.Typename == "StatusContext" {
		switch n.State {
		case "SUCCESS":
			return domain.CheckSuccess
		case "FAILURE", "ERROR":
			return domain.CheckFailure
		default:
			return domain.CheckPending
		}
	}
	if n.Status != "COMPLETED" {
		return domain.CheckRunning
	}
	switch n.Conclusion {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return domain.CheckSuccess
	case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return domain.CheckFailure
	default:
		return domain.CheckPending
	}
}

func toLogLine(l github.LogLine) domain.LogLine {
	return domain.LogLine{Step: l.Step, Time: l.Time, Text: l.Text}
}

// fromRerunScope spells a rerun scope the way the shared github package
// does, which is what RerunWorkflow's two implementations take.
func fromRerunScope(s domain.RerunScope) github.RerunScope {
	if s == domain.RerunAll {
		return github.RerunAll
	}
	return github.RerunFailed
}
