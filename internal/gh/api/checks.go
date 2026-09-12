package api

import (
	"context"
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
