// Package gh turns what a GitHub client answers into the domain's values,
// and is what the usecase layer's ports are satisfied by.
//
// Gateway embeds backend anonymously. A method this package has not
// converted yet is promoted from the embedded value unchanged; a converted
// one is shadowed by an explicit method whose signature speaks the domain.
// That is what lets the conversion land one port group at a time.
// StartReview and SubmitNewReview stay off the usecase-facing port for
// good; review.go calls them through g.backend rather than by promotion.
package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

type itemFetcher interface {
	GetPR(ctx context.Context, repo string, number int) (gql.PullRequest, error)
	GetIssue(ctx context.Context, repo string, number int) (gql.Issue, error)
}

type commenter interface {
	AddPRComment(ctx context.Context, repo string, number int, body string) error
	AddIssueComment(ctx context.Context, repo string, number int, body string) error
}

type stateChanger interface {
	ClosePR(ctx context.Context, repo string, number int) error
	ReopenPR(ctx context.Context, repo string, number int) error
	CloseIssue(ctx context.Context, repo string, number int) error
	ReopenIssue(ctx context.Context, repo string, number int) error
}

type labelEditor interface {
	EditPRLabels(ctx context.Context, repo string, number int, add, remove []string) error
	EditIssueLabels(ctx context.Context, repo string, number int, add, remove []string) error
}

type assigneeEditor interface {
	EditPRAssignees(ctx context.Context, repo string, number int, add, remove []string) error
	EditIssueAssignees(ctx context.Context, repo string, number int, add, remove []string) error
}

type lister interface {
	ListPRs(ctx context.Context, repo string) ([]gql.PullRequest, error)
	ListIssues(ctx context.Context, repo string) ([]gql.Issue, error)
	RepoName(ctx context.Context) (string, error)
	ListLabels(ctx context.Context, repo string) ([]gql.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

// viewerFetcher names the signed-in user. Gateway promotes it unchanged:
// there is nothing to convert -- a login is a string in both languages.
// Promoting it is safe under the rule at the top of this file because the
// only caller (root's resolveViewer) drops the failure rather than showing
// it on the fatal-error screen.
type viewerFetcher interface {
	Viewer(ctx context.Context) (string, error)
}

// crossRepoLister is what an operation that cannot name a single repository
// takes: unlike lister's operations, none of these are "the contents of one
// named repository". Which columns the Work board has, and what each one
// means, is the gateway's own knowledge: the backend only runs a search.
type crossRepoLister interface {
	SearchItems(ctx context.Context, query string) ([]gql.SearchItem, error)
	RepoCounts(ctx context.Context, repos []string) ([]gql.RepoCount, error)
}

// repoFinder is what the add dialog offers: candidates while it is typed
// into, and the repositories a first run can be seeded from.
type repoFinder interface {
	SearchRepos(ctx context.Context, query string, limit int) ([]github.Repository, error)
	ListOwnRepos(ctx context.Context, owner string, limit int) ([]github.Repository, error)
	ListOrgs(ctx context.Context) ([]string, error)
}

type reviewFetcher interface {
	PRDiff(ctx context.Context, repo string, number int) (github.Diff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gql.ReviewContext, error)
}

type reviewer interface {
	StartReview(ctx context.Context, pullRequestID string) (string, error)
	AddReviewThread(ctx context.Context, reviewID string, c gql.PendingComment) error
	SubmitReview(ctx context.Context, reviewID string, event gql.ReviewEvent, body string) error
	SubmitNewReview(ctx context.Context, pullRequestID string, event gql.ReviewEvent, body string) error
	DiscardReview(ctx context.Context, reviewID string) error
}

type checksFetcher interface {
	PRChecks(ctx context.Context, repo string, number int) ([]gql.CheckRun, error)
	JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]github.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, runID int64, scope github.RerunScope) error
}

type merger interface {
	PRMergeContext(ctx context.Context, repo string, number int) (gql.MergeContext, error)
	MergePR(ctx context.Context, pullRequestID string, method gql.MergeMethod) error
	EnableAutoMerge(ctx context.Context, pullRequestID string, method gql.MergeMethod) error
	DisableAutoMerge(ctx context.Context, pullRequestID string) error
}

// backend is what a GitHub client answers. It is declared here, on the
// consumer's side, rather than exported by the client packages.
//
// A method Gateway promotes rather than overrides returns the client's own
// sentinels, unrecognised by domain.IsFatal. That is safe only as long as no
// promoted method's failure reaches the fatal-error screen -- today those
// failures are shown inline or discarded instead. If one is ever wired to
// that screen, give it an override that calls wrap.
type backend interface {
	itemFetcher
	commenter
	stateChanger
	labelEditor
	assigneeEditor
	lister
	viewerFetcher
	crossRepoLister
	repoFinder
	reviewFetcher
	reviewer
	checksFetcher
	merger
}
