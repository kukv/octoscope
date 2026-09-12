package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kukv/octoscope/internal/gh"
)

// RerunWorkflow starts a workflow run again. GitHub has two endpoints rather
// than one with a flag: the failed-jobs one leaves the jobs that passed alone.
func (c *Client) RerunWorkflow(ctx context.Context, repo string, runID int64, scope gh.RerunScope) error {
	r, err := c.repoPath(repo)
	if err != nil {
		return err
	}
	endpoint := "rerun"
	if scope == gh.RerunFailed {
		endpoint = "rerun-failed-jobs"
	}
	path := fmt.Sprintf("repos/%s/actions/runs/%d/%s", r, runID, endpoint)
	_, err = c.write(ctx, http.MethodPost, path, nil)
	return err
}

// job is one Actions job. Only the fields the log needs are decoded: which run
// carries the archive, what the zip's directory is named after, and which
// steps failed.
type job struct {
	ID         int64     `json:"id"`
	RunID      int64     `json:"run_id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	Conclusion string    `json:"conclusion"`
	Steps      []jobStep `json:"steps"`
}

type jobStep struct {
	Name       string `json:"name"`
	Number     int    `json:"number"`
	Conclusion string `json:"conclusion"`
}

// failed is what GitHub reports for work that did not succeed. action_required
// and timed_out are failures the same way an outright failure is: each one is
// a red mark someone clicked to read.
func failed(conclusion string) bool {
	switch conclusion {
	case "failure", "startup_failure", "timed_out", "action_required":
		return true
	}
	return false
}

// JobLog reads one job's log. With failedOnly it answers with the failed steps
// alone: that is what someone opening a red check came to read.
//
// A job that has not finished has no log yet, and a passing job has no failed
// steps: neither is an error, and the second answers with no lines at all.
func (c *Client) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	body, err := c.read(ctx, fmt.Sprintf("repos/%s/actions/jobs/%d", r, jobID), "")
	if err != nil {
		return nil, err
	}
	var j job
	if err := json.Unmarshal(body, &j); err != nil {
		return nil, fmt.Errorf("read job %d: %w", jobID, err)
	}
	if j.Status != "completed" {
		return nil, fmt.Errorf("job %d is still in progress; logs will be available when it is complete", jobID)
	}
	if failedOnly && !failed(j.Conclusion) {
		return nil, nil
	}
	return c.jobLogLines(ctx, r, j, failedOnly)
}
