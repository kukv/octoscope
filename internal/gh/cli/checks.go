package cli

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// pageInfo is gone with the documents; review.go follows in the next step.
type pageInfo = gql.PageInfo

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
