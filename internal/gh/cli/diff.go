package cli

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
)

// PRDiff returns the pull request's diff, one entry per file.
//
// --color never is passed explicitly: gh colours its output when it thinks a
// terminal is watching, and escape sequences in the middle of a line would
// break both the parser and every width calculation downstream.
//
// gh pr diff refuses past 300 files (HTTP 406); the files API has no such
// limit. Rather than matching GitHub's error text for that (its wording is
// not ours to depend on, and .claude/rules/errors.md says not to branch on
// error strings), any failure here is retried through the files API, which
// is cheap since failures are rare. If that also fails, both errors are
// joined rather than one discarding the other: this one still describes
// what the user actually asked for, but a bug in the fallback itself must
// not go unseen either.
func (c *Client) PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	args := appendRepo(
		[]string{"pr", "diff", strconv.Itoa(number), "--color", "never"},
		c.effectiveRepo(repo),
	)
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		files, ferr := c.prFiles(ctx, repo, number)
		if ferr == nil {
			return files, nil
		}
		// err is the one that describes what the user actually asked for and
		// is reported as such (see showError() in internal/tui/app); ferr is
		// joined in rather than discarded so a bug in prFiles itself (a
		// parse failure, say) is not silently swallowed
		// (.claude/rules/errors.md). errors.Is still matches err through the
		// join.
		return nil, errors.Join(err, ferr)
	}
	return gh.ParseDiff(out), nil
}

// prFiles is the files-API fallback for PRDiff. --paginate is required: the
// endpoint's default page is 30 files, and the pull request this fallback
// exists for had 418. per_page=100 cuts that down to 5 requests instead of
// 14 (ListAssignees in cli.go does the same for its own listing).
func (c *Client) prFiles(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	out, err := c.read(ctx, c.dir, "api", prFilesPath(c.effectiveRepo(repo), number), "--paginate")
	if err != nil {
		return nil, err
	}
	return gh.ParseFilesAPI(out)
}

// prFilesPath builds the REST path for the files-API fallback. gh api
// substitutes {owner}/{repo} from the current directory's repo when no
// repository is named (see ListAssignees); an override is spelled out
// explicitly instead, because gh api takes no --repo flag.
func prFilesPath(repo string, number int) string {
	repoPart := "{owner}/{repo}"
	if repo != "" {
		repoPart = repo
	}
	return fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100", repoPart, number)
}
