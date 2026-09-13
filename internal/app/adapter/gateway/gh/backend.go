// Package gh turns what a GitHub client answers into the domain's values,
// and is what the usecase layer's ports are satisfied by.
//
// Gateway embeds backend anonymously. A method this package has not
// converted yet is promoted from the embedded value unchanged; a converted
// one is shadowed by an explicit method whose signature speaks the domain.
// That is what lets the conversion land one port group at a time.
package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

type itemFetcher interface {
	GetPR(ctx context.Context, repo string, number int) (gql.PullRequest, error)
	GetIssue(ctx context.Context, repo string, number int) (gql.Issue, error)
}

type commenter interface {
	AddPRComment(repo string, number int, body string) error
	AddIssueComment(repo string, number int, body string) error
}

type stateChanger interface {
	ClosePR(repo string, number int) error
	ReopenPR(repo string, number int) error
	CloseIssue(repo string, number int) error
	ReopenIssue(repo string, number int) error
}

type labelEditor interface {
	EditPRLabels(repo string, number int, add, remove []string) error
	EditIssueLabels(repo string, number int, add, remove []string) error
}

type assigneeEditor interface {
	EditPRAssignees(repo string, number int, add, remove []string) error
	EditIssueAssignees(repo string, number int, add, remove []string) error
}

type lister interface {
	ListPRs(ctx context.Context, repo string) ([]gql.PullRequest, error)
	ListIssues(ctx context.Context, repo string) ([]gql.Issue, error)
	RepoName(ctx context.Context) (string, error)
	ListLabels(ctx context.Context, repo string) ([]gql.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
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
	PRDiff(ctx context.Context, repo string, number int) ([]domain.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error)
}

type reviewer interface {
	StartReview(pullRequestID string) (string, error)
	AddReviewThread(reviewID string, c domain.PendingComment) error
	SubmitReview(reviewID string, event domain.ReviewEvent, body string) error
	SubmitNewReview(pullRequestID string, event domain.ReviewEvent, body string) error
	DiscardReview(reviewID string) error
}

type opener interface {
	OpenWeb(url string) error
}

type checksFetcher interface {
	PRChecks(ctx context.Context, repo string, number int) (domain.Checks, error)
	JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]domain.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, runID int64, scope domain.RerunScope) error
}

type merger interface {
	PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error)
	MergePR(pullRequestID string, method domain.MergeMethod) error
	EnableAutoMerge(pullRequestID string, method domain.MergeMethod) error
	DisableAutoMerge(pullRequestID string) error
}

// backend is what a GitHub client answers. It is declared here, on the
// consumer's side, rather than exported by the client packages.
type backend interface {
	itemFetcher
	commenter
	stateChanger
	labelEditor
	assigneeEditor
	lister
	crossRepoLister
	repoFinder
	reviewFetcher
	reviewer
	opener
	checksFetcher
	merger
}
