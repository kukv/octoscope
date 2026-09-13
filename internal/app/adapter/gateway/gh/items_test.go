package gh

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// fakeBackend answers whichever method a test set a function for. Embedding
// the nil backend interface, rather than stubbing every method by hand,
// panics loudly if a test calls a method it did not mean to exercise.
type fakeBackend struct {
	backend
	getPR           func(ctx context.Context, repo string, number int) (gql.PullRequest, error)
	getIssue        func(ctx context.Context, repo string, number int) (gql.Issue, error)
	prDiff          func(ctx context.Context, repo string, number int) (github.Diff, error)
	prReviewContext func(ctx context.Context, repo string, number int) (gql.ReviewContext, error)
	addReviewThread func(reviewID string, c gql.PendingComment) error
	submitReview    func(reviewID string, event gql.ReviewEvent, body string) error
	submitNewReview func(pullRequestID string, event gql.ReviewEvent, body string) error
}

func (f fakeBackend) GetPR(ctx context.Context, repo string, number int) (gql.PullRequest, error) {
	return f.getPR(ctx, repo, number)
}

func (f fakeBackend) GetIssue(ctx context.Context, repo string, number int) (gql.Issue, error) {
	return f.getIssue(ctx, repo, number)
}

// TestGetPRTranslatesTheWireShapeIntoTheDomain gives every field of
// gql.PullRequest a distinct, non-zero value and compares the whole
// resulting domain.PR against a fully written-out expectation. A field the
// conversion forgot to copy is left at its zero value, which a struct-wide
// comparison catches; asserting a handful of fields would not.
func TestGetPRTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	commentedAt := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	node := gql.PullRequest{
		Number:         59,
		Title:          "add the gateway",
		State:          "OPEN",
		URL:            "https://github.com/kukv/octoscope/pull/59",
		IsDraft:        true,
		UpdatedAt:      updatedAt,
		ReviewDecision: "APPROVED",
		HeadRefName:    "refactor/pr2b-gateway",
		BaseRefName:    "main",
		Additions:      42,
		Deletions:      7,
		Body:           "this adds the gateway",
		Author:         gql.Author{Login: "kukv"},
	}
	node.Labels.Nodes = []gql.Label{
		{Name: "bug", Color: "d73a4a"},
		{Name: "wip", Color: "ededed"},
	}
	node.Assignees.Nodes = []gql.Author{
		{Login: "octocat"},
		{Login: "reviewer"},
	}
	node.Comments.Nodes = []gql.Comment{
		{Author: gql.Author{Login: "octocat"}, Body: "lgtm", CreatedAt: commentedAt},
	}
	// node.Commits.Nodes is an anonymous struct too deeply nested to build
	// field by field, so it is filled the way the real client fills it: by
	// decoding it out of JSON in the shape the roll-up query returns.
	const commits = `[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
		{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS"},
		{"__typename":"StatusContext","context":"ci/deploy","state":"FAILURE"}
	]}}}}]`
	if err := json.Unmarshal([]byte(commits), &node.Commits.Nodes); err != nil {
		t.Fatalf("build commits fixture: %v", err)
	}

	g := New(fakeBackend{getPR: func(context.Context, string, int) (gql.PullRequest, error) {
		return node, nil
	}})

	pr, err := g.GetPR(context.Background(), "kukv/octoscope", 59)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}

	want := domain.PR{
		Number:  59,
		Title:   "add the gateway",
		Author:  domain.Author{Login: "kukv"},
		State:   domain.StateOpen,
		IsDraft: true,
		Review:  domain.ReviewApproved,
		URL:     "https://github.com/kukv/octoscope/pull/59",
		Body:    "this adds the gateway",
		Comments: []domain.Comment{
			{Author: domain.Author{Login: "octocat"}, Body: "lgtm", CreatedAt: commentedAt},
		},
		Labels: []domain.Label{
			{Name: "bug", Color: "d73a4a"},
			{Name: "wip", Color: "ededed"},
		},
		Assignees: []domain.Author{
			{Login: "octocat"},
			{Login: "reviewer"},
		},
		Checks: domain.Checks{
			Total:  2,
			Passed: 1,
			Failed: 1,
			State:  domain.CheckFailure,
			Runs: []domain.CheckRun{
				{Name: "build", State: domain.CheckSuccess, Kind: domain.CheckKindRun},
				{Name: "ci/deploy", State: domain.CheckFailure, Kind: domain.CheckKindStatus},
			},
		},
		UpdatedAt: updatedAt,
		Head:      "refactor/pr2b-gateway",
		Base:      "main",
		Additions: 42,
		Deletions: 7,
	}
	if !reflect.DeepEqual(pr, want) {
		t.Errorf("GetPR() = %+v, want %+v", pr, want)
	}
}

// TestGetIssueTranslatesTheWireShapeIntoTheDomain gives every field of
// gql.Issue a distinct, non-zero value and compares the whole resulting
// domain.Issue against a fully written-out expectation, for the same reason
// as TestGetPRTranslatesTheWireShapeIntoTheDomain above.
func TestGetIssueTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	commentedAt := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)

	node := gql.Issue{
		Number:    54,
		Title:     "track the conversion",
		State:     "CLOSED",
		URL:       "https://github.com/kukv/octoscope/issues/54",
		UpdatedAt: updatedAt,
		Body:      "conversion tests only check a handful of fields",
		Author:    gql.Author{Login: "kukv"},
	}
	node.Labels.Nodes = []gql.Label{
		{Name: "docs", Color: "0075ca"},
		{Name: "wip", Color: "ededed"},
	}
	node.Assignees.Nodes = []gql.Author{
		{Login: "octocat"},
		{Login: "reviewer"},
	}
	node.Comments.Nodes = []gql.Comment{
		{Author: gql.Author{Login: "octocat"}, Body: "done", CreatedAt: commentedAt},
	}

	g := New(fakeBackend{getIssue: func(context.Context, string, int) (gql.Issue, error) {
		return node, nil
	}})

	issue, err := g.GetIssue(context.Background(), "kukv/octoscope", 54)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}

	want := domain.Issue{
		Number:    54,
		Title:     "track the conversion",
		Author:    domain.Author{Login: "kukv"},
		State:     domain.StateClosed,
		UpdatedAt: updatedAt,
		URL:       "https://github.com/kukv/octoscope/issues/54",
		Body:      "conversion tests only check a handful of fields",
		Comments: []domain.Comment{
			{Author: domain.Author{Login: "octocat"}, Body: "done", CreatedAt: commentedAt},
		},
		Labels: []domain.Label{
			{Name: "docs", Color: "0075ca"},
			{Name: "wip", Color: "ededed"},
		},
		Assignees: []domain.Author{
			{Login: "octocat"},
			{Login: "reviewer"},
		},
	}
	if !reflect.DeepEqual(issue, want) {
		t.Errorf("GetIssue() = %+v, want %+v", issue, want)
	}
}
