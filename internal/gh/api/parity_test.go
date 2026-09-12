package api_test

import (
	"context"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/api"
	"github.com/kukv/octoscope/internal/gh/cli"
)

// graphQLSource is every GraphQL-backed operation the usecase layer's source
// interface asks for. The rest of that interface -- the REST calls and the
// Actions calls -- arrives in later slices; this is the part this backend is
// finished for.
type graphQLSource interface {
	ListPRs(ctx context.Context, repo string) ([]gh.PR, error)
	ListIssues(ctx context.Context, repo string) ([]gh.Issue, error)
	GetPR(ctx context.Context, repo string, number int) (gh.PR, error)
	GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error)
	RepoName(ctx context.Context) (string, error)
	ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error)
	SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)
	RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)
	PRChecks(ctx context.Context, repo string, number int) (gh.Checks, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
	PRMergeContext(ctx context.Context, repo string, number int) (gh.MergeContext, error)
	OpenWeb(url string) error
}

// Both backends answer the same operations with the same domain types. A
// method that only one of them has would leave the other's screens empty.
var (
	_ graphQLSource = (*api.Client)(nil)
	_ graphQLSource = (*cli.Client)(nil)
)

// restSource is the part of the usecase layer's source interface that REST
// answers. Both backends satisfy it with the same domain types: a method only
// one of them has would leave the other's screens empty.
//
// Actions (RerunWorkflow, JobLog) are the last group and arrive in the next
// slice; this file goes away then, when usecase.New(api.New(...)) compiles
// and the compiler itself becomes the parity check.
type restWriter interface {
	AddPRComment(repo string, number int, body string) error
	AddIssueComment(repo string, number int, body string) error
	ClosePR(repo string, number int) error
	ReopenPR(repo string, number int) error
	CloseIssue(repo string, number int) error
	ReopenIssue(repo string, number int) error
}

type restEditor interface {
	EditPRLabels(repo string, number int, add, remove []string) error
	EditIssueLabels(repo string, number int, add, remove []string) error
	EditPRAssignees(repo string, number int, add, remove []string) error
	EditIssueAssignees(repo string, number int, add, remove []string) error
}

type restReader interface {
	ListLabels(ctx context.Context, repo string) ([]gh.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error)
	ListOwnRepos(ctx context.Context, owner string, limit int) ([]gh.RepoCandidate, error)
	ListOrgs(ctx context.Context) ([]string, error)
}

type restSource interface {
	restWriter
	restEditor
	restReader
}

var (
	_ restSource = (*api.Client)(nil)
	_ restSource = (*cli.Client)(nil)
)
