package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
)

// diffMediaType is what asks the pull request endpoint for a unified diff
// rather than the pull request's own fields.
const diffMediaType = "application/vnd.github.v3.diff"

// PRDiff returns the pull request's diff, one entry per file.
//
// GitHub refuses a diff past a few hundred files; the files API has no such
// limit. Rather than matching GitHub's error text for that (its wording is
// not ours to depend on), any failure here is retried through the files API,
// which is cheap since failures are rare. If that also fails, both errors are
// joined rather than one discarding the other: this one still describes what
// the user actually asked for, but a bug in the fallback itself must not go
// unseen either.
func (c *Client) PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	r, err := c.repoPath(repo)
	if err != nil {
		return nil, err
	}
	out, derr := c.read(ctx, fmt.Sprintf("repos/%s/pulls/%d", r, number), diffMediaType)
	if derr != nil {
		files, ferr := c.prFiles(ctx, r, number)
		if ferr == nil {
			return files, nil
		}
		return nil, errors.Join(derr, ferr)
	}
	return gh.ParseDiff(out), nil
}

// prFiles is the files-API fallback for PRDiff. It walks every page: the
// endpoint's default page is 30 files, and the pull request this fallback
// exists for had 418.
func (c *Client) prFiles(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	path := fmt.Sprintf("repos/%s/pulls/%d/files?per_page=%d", repo, number, pageSize)
	var files []gh.FileDiff
	err := c.walkPages(ctx, path, func(out []byte) error {
		page, err := gh.ParseFilesAPI(out)
		if err != nil {
			return err
		}
		files = append(files, page...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
