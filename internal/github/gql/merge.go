package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
)

//go:embed merge.graphql
var mergeContextQuery string

// MergeContext is what the merge popup needs, as GraphQL answers it.
type MergeContext struct {
	PullRequestID            string
	IsDraft                  bool
	Mergeable                string
	MergeStateStatus         string
	ReviewDecision           string
	SquashMergeAllowed       bool
	MergeCommitAllowed       bool
	RebaseMergeAllowed       bool
	DeleteBranchOnMerge      bool
	AutoMergeAllowed         bool
	ViewerCanEnableAutoMerge bool
	ViewerPermission         string
	AutoMergeEnabled         bool
}

type mergeContextResponse struct {
	Data struct {
		Repository struct {
			SquashMergeAllowed  bool   `json:"squashMergeAllowed"`
			MergeCommitAllowed  bool   `json:"mergeCommitAllowed"`
			RebaseMergeAllowed  bool   `json:"rebaseMergeAllowed"`
			DeleteBranchOnMerge bool   `json:"deleteBranchOnMerge"`
			AutoMergeAllowed    bool   `json:"autoMergeAllowed"`
			ViewerPermission    string `json:"viewerPermission"`
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
func (c *Client) PRMergeContext(ctx context.Context, repo string, number int) (MergeContext, error) {
	repoFields, err := c.repoVars(repo)
	if err != nil {
		return MergeContext{}, err
	}
	vars := append(slices.Clone(repoFields), N("number", number))
	out, err := c.Read(ctx, mergeContextQuery, vars...)
	if err != nil {
		return MergeContext{}, err
	}
	var resp mergeContextResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return MergeContext{}, fmt.Errorf("parse merge context: %w", err)
	}
	r := resp.Data.Repository
	pr := r.PullRequest
	return MergeContext{
		PullRequestID:            pr.ID,
		IsDraft:                  pr.IsDraft,
		Mergeable:                pr.Mergeable,
		MergeStateStatus:         pr.MergeStateStatus,
		ReviewDecision:           pr.ReviewDecision,
		SquashMergeAllowed:       r.SquashMergeAllowed,
		MergeCommitAllowed:       r.MergeCommitAllowed,
		RebaseMergeAllowed:       r.RebaseMergeAllowed,
		DeleteBranchOnMerge:      r.DeleteBranchOnMerge,
		AutoMergeAllowed:         r.AutoMergeAllowed,
		ViewerCanEnableAutoMerge: pr.ViewerCanEnableAutoMerge,
		ViewerPermission:         r.ViewerPermission,
		AutoMergeEnabled:         pr.AutoMergeRequest != nil,
	}, nil
}

//go:embed merge_pr.graphql
var mergePRMutation string

//go:embed enable_auto_merge.graphql
var enableAutoMergeMutation string

//go:embed disable_auto_merge.graphql
var disableAutoMergeMutation string

// MergeMethod spells a merge method the way the GraphQL
// PullRequestMergeMethod enum does.
type MergeMethod string

const (
	MergeMethodSquash MergeMethod = "SQUASH"
	MergeMethodMerge  MergeMethod = "MERGE"
	MergeMethodRebase MergeMethod = "REBASE"
)

// The three mutations take no context, for the same reason review.go's do:
// a merge that has happened has happened.

// MergePR merges the pull request now.
func (c *Client) MergePR(pullRequestID string, method MergeMethod) error {
	_, err := c.Write(context.Background(), mergePRMutation,
		S("pullRequestId", pullRequestID),
		S("mergeMethod", string(method)),
	)
	return err
}

// EnableAutoMerge asks GitHub to merge the pull request once what it is
// waiting on is in.
func (c *Client) EnableAutoMerge(pullRequestID string, method MergeMethod) error {
	_, err := c.Write(context.Background(), enableAutoMergeMutation,
		S("pullRequestId", pullRequestID),
		S("mergeMethod", string(method)),
	)
	return err
}

// DisableAutoMerge cancels a queued auto-merge.
func (c *Client) DisableAutoMerge(pullRequestID string) error {
	_, err := c.Write(context.Background(), disableAutoMergeMutation, S("pullRequestId", pullRequestID))
	return err
}
