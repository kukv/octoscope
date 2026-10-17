package cli

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/kukv/octoscope/internal/github"
)

// JobLog reads one job's log. With failedOnly it asks for the failed steps
// alone: that is what someone opening a red check came to read, and a whole
// job's log has been measured at nine times the length.
//
// gh exits zero with no output at all when a successful job is asked for its
// failed steps, so an empty result is an answer, not an error.
func (c *Client) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]github.LogLine, error) {
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

// caretSGR is a colour code the way gh hands one over: gh turns the escape
// character into caret notation, so ESC [ 36;1 m arrives as "^[[36;1m".
var caretSGR = regexp.MustCompile(`\^\[\[[0-9;]*m`)

// parseJobLog reads the format gh prints: three tab-separated fields, the
// job's name, the step's name, and the line the runner wrote.
//
// Colour codes are dropped: the runner puts one around every command it
// echoes, and on screen that is only noise. The api backend drops them the
// same way.
func parseJobLog(out string) []github.LogLine {
	var lines []github.LogLine
	for _, raw := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if raw == "" {
			continue
		}
		fields := strings.SplitN(raw, "\t", 3)
		if len(fields) < 3 {
			lines = append(lines, github.LogLine{Text: caretSGR.ReplaceAllString(fields[len(fields)-1], "")})
			continue
		}
		lines = append(lines, github.ParseLogLine(caretSGR.ReplaceAllString(fields[1], ""), caretSGR.ReplaceAllString(fields[2], "")))
	}
	return lines
}

// RerunWorkflow starts a workflow run again. GraphQL has no mutation for
// this, which is why it goes through the subcommand.
func (c *Client) RerunWorkflow(ctx context.Context, repo string, runID int64, scope github.RerunScope) error {
	args := []string{"run", "rerun", strconv.FormatInt(runID, 10)}
	if scope == github.RerunFailed {
		args = append(args, "--failed")
	}
	_, err := c.run(ctx, c.dir, appendRepo(args, c.effectiveRepo(repo))...)
	return err
}
