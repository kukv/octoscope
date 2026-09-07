package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed merge.graphql
var mergeContextQuery string

type mergeContextResponse struct {
	Data struct {
		Repository struct {
			SquashMergeAllowed  bool `json:"squashMergeAllowed"`
			MergeCommitAllowed  bool `json:"mergeCommitAllowed"`
			RebaseMergeAllowed  bool `json:"rebaseMergeAllowed"`
			DeleteBranchOnMerge bool `json:"deleteBranchOnMerge"`
			AutoMergeAllowed    bool `json:"autoMergeAllowed"`
			PullRequest         struct {
				ID                       string `json:"id"`
				IsDraft                  bool   `json:"isDraft"`
				Mergeable                string `json:"mergeable"`
				MergeStateStatus         string `json:"mergeStateStatus"`
				ReviewDecision           string `json:"reviewDecision"`
				ViewerCanEnableAutoMerge bool   `json:"viewerCanEnableAutoMerge"`
				AutoMergeRequest         *struct {
					EnabledAt string `json:"enabledAt"`
				} `json:"autoMergeRequest"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// PRMergeContext fetches what the merge popup draws: what the repository
// allows and what state this pull request is in.
func (c *Client) PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error) {
	repoFields, err := repoArgs(c.effectiveRepo(repo))
	if err != nil {
		return gh.MergeContext{}, err
	}
	args := append([]string{"api", "graphql", "-f", "query=" + mergeContextQuery}, repoFields...)
	args = append(args, "-F", "number="+strconv.Itoa(number))
	out, err := c.run(ctx, c.dir, args...)
	if err != nil {
		return gh.MergeContext{}, err
	}
	var resp mergeContextResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.MergeContext{}, fmt.Errorf("parse merge context: %w", err)
	}
	r := resp.Data.Repository
	pr := r.PullRequest
	return gh.MergeContext{
		PullRequestID:            pr.ID,
		IsDraft:                  pr.IsDraft,
		Mergeable:                parseMergeable(pr.Mergeable),
		State:                    parseMergeState(pr.MergeStateStatus),
		Review:                   gh.ParseReviewDecision(pr.ReviewDecision),
		Methods:                  allowedMethods(r.SquashMergeAllowed, r.MergeCommitAllowed, r.RebaseMergeAllowed),
		DeleteBranchOnMerge:      r.DeleteBranchOnMerge,
		AutoMergeAllowed:         r.AutoMergeAllowed,
		ViewerCanEnableAutoMerge: pr.ViewerCanEnableAutoMerge,
		AutoMergeEnabled:         pr.AutoMergeRequest != nil,
	}, nil
}

// allowedMethods lists the methods in the order the popup draws them
// (standalone design §4.4.4): squash, merge commit, rebase.
func allowedMethods(squash, commit, rebase bool) []gh.MergeMethod {
	var methods []gh.MergeMethod
	if squash {
		methods = append(methods, gh.MergeSquash)
	}
	if commit {
		methods = append(methods, gh.MergeCommit)
	}
	if rebase {
		methods = append(methods, gh.MergeRebase)
	}
	return methods
}

// A value neither of these knows is read as "unknown" rather than failing
// the fetch: GitHub adds values to these enums.
func parseMergeable(s string) gh.Mergeable {
	switch s {
	case "MERGEABLE":
		return gh.MergeableYes
	case "CONFLICTING":
		return gh.MergeableConflicting
	}
	return gh.MergeableUnknown
}

func parseMergeState(s string) gh.MergeState {
	switch s {
	case "CLEAN":
		return gh.MergeStateClean
	case "BLOCKED":
		return gh.MergeStateBlocked
	case "BEHIND":
		return gh.MergeStateBehind
	case "DIRTY":
		return gh.MergeStateDirty
	case "UNSTABLE":
		return gh.MergeStateUnstable
	case "HAS_HOOKS":
		return gh.MergeStateHasHooks
	}
	return gh.MergeStateUnknown
}

//go:embed merge_pr.graphql
var mergePRMutation string

//go:embed enable_auto_merge.graphql
var enableAutoMergeMutation string

//go:embed disable_auto_merge.graphql
var disableAutoMergeMutation string

// apiMergeMethod spells a method the way the GraphQL PullRequestMergeMethod
// enum does. It is the one place that knows those words
// (.claude/rules/architecture.md).
func apiMergeMethod(m gh.MergeMethod) string {
	switch m {
	case gh.MergeCommit:
		return "MERGE"
	case gh.MergeRebase:
		return "REBASE"
	default:
		return "SQUASH"
	}
}

// The three mutations take no context, for the same reason review.go's do:
// a merge that has happened has happened.

// MergePR merges the pull request now.
func (c *Client) MergePR(pullRequestID string, method gh.MergeMethod) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+mergePRMutation,
		"-f", "pullRequestId="+pullRequestID,
		"-f", "mergeMethod="+apiMergeMethod(method),
	)
	return err
}

// EnableAutoMerge asks GitHub to merge the pull request once what it is
// waiting on is in.
func (c *Client) EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+enableAutoMergeMutation,
		"-f", "pullRequestId="+pullRequestID,
		"-f", "mergeMethod="+apiMergeMethod(method),
	)
	return err
}

// DisableAutoMerge cancels a queued auto-merge.
func (c *Client) DisableAutoMerge(pullRequestID string) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+disableAutoMergeMutation,
		"-f", "pullRequestId="+pullRequestID,
	)
	return err
}
