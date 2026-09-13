package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kukv/octoscope/internal/app/domain"
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
func (c *Client) PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error) {
	repoFields, err := c.repoVars(repo)
	if err != nil {
		return domain.MergeContext{}, err
	}
	vars := append(slices.Clone(repoFields), N("number", number))
	out, err := c.Read(ctx, mergeContextQuery, vars...)
	if err != nil {
		return domain.MergeContext{}, err
	}
	var resp mergeContextResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return domain.MergeContext{}, fmt.Errorf("parse merge context: %w", err)
	}
	r := resp.Data.Repository
	pr := r.PullRequest
	return domain.MergeContext{
		PullRequestID:            pr.ID,
		IsDraft:                  pr.IsDraft,
		Mergeable:                parseMergeable(pr.Mergeable),
		State:                    parseMergeState(pr.MergeStateStatus),
		Review:                   domain.ParseReviewDecision(pr.ReviewDecision),
		Methods:                  allowedMethods(r.SquashMergeAllowed, r.MergeCommitAllowed, r.RebaseMergeAllowed),
		DeleteBranchOnMerge:      r.DeleteBranchOnMerge,
		AutoMergeAllowed:         r.AutoMergeAllowed,
		ViewerCanEnableAutoMerge: pr.ViewerCanEnableAutoMerge,
		AutoMergeEnabled:         pr.AutoMergeRequest != nil,
	}, nil
}

// allowedMethods lists the methods in the order the popup draws them
// (standalone design §4.4.4): squash, merge commit, rebase.
func allowedMethods(squash, commit, rebase bool) []domain.MergeMethod {
	var methods []domain.MergeMethod
	if squash {
		methods = append(methods, domain.MergeSquash)
	}
	if commit {
		methods = append(methods, domain.MergeCommit)
	}
	if rebase {
		methods = append(methods, domain.MergeRebase)
	}
	return methods
}

// A value neither of these knows is read as "unknown" rather than failing
// the fetch: GitHub adds values to these enums.
func parseMergeable(s string) domain.Mergeable {
	switch s {
	case "MERGEABLE":
		return domain.MergeableYes
	case "CONFLICTING":
		return domain.MergeableConflicting
	}
	return domain.MergeableUnknown
}

func parseMergeState(s string) domain.MergeState {
	switch s {
	case "CLEAN":
		return domain.MergeStateClean
	case "BLOCKED":
		return domain.MergeStateBlocked
	case "BEHIND":
		return domain.MergeStateBehind
	case "DIRTY":
		return domain.MergeStateDirty
	case "UNSTABLE":
		return domain.MergeStateUnstable
	case "HAS_HOOKS":
		return domain.MergeStateHasHooks
	}
	return domain.MergeStateUnknown
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
func apiMergeMethod(m domain.MergeMethod) string {
	switch m {
	case domain.MergeCommit:
		return "MERGE"
	case domain.MergeRebase:
		return "REBASE"
	default:
		return "SQUASH"
	}
}

// The three mutations take no context, for the same reason review.go's do:
// a merge that has happened has happened.

// MergePR merges the pull request now.
func (c *Client) MergePR(pullRequestID string, method domain.MergeMethod) error {
	_, err := c.Write(context.Background(), mergePRMutation,
		S("pullRequestId", pullRequestID),
		S("mergeMethod", apiMergeMethod(method)),
	)
	return err
}

// EnableAutoMerge asks GitHub to merge the pull request once what it is
// waiting on is in.
func (c *Client) EnableAutoMerge(pullRequestID string, method domain.MergeMethod) error {
	_, err := c.Write(context.Background(), enableAutoMergeMutation,
		S("pullRequestId", pullRequestID),
		S("mergeMethod", apiMergeMethod(method)),
	)
	return err
}

// DisableAutoMerge cancels a queued auto-merge.
func (c *Client) DisableAutoMerge(pullRequestID string) error {
	_, err := c.Write(context.Background(), disableAutoMergeMutation, S("pullRequestId", pullRequestID))
	return err
}
