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
	prChecks        func(ctx context.Context, repo string, number int) ([]gql.CheckRun, error)
	jobLog          func(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]github.LogLine, error)
	rerunWorkflow   func(ctx context.Context, repo string, runID int64, scope github.RerunScope) error
	listPRs         func(ctx context.Context, repo string) ([]gql.PullRequest, error)
	listIssues      func(ctx context.Context, repo string) ([]gql.Issue, error)
	listLabels      func(ctx context.Context, repo string) ([]gql.Label, error)
	prMergeContext  func(ctx context.Context, repo string, number int) (gql.MergeContext, error)
	mergePR         func(pullRequestID string, method gql.MergeMethod) error
	enableAutoMerge func(pullRequestID string, method gql.MergeMethod) error
	searchRepos     func(ctx context.Context, query string, limit int) ([]github.Repository, error)
	listOwnRepos    func(ctx context.Context, owner string, limit int) ([]github.Repository, error)
	searchItems     func(ctx context.Context, query string) ([]gql.SearchItem, error)
	repoCounts      func(ctx context.Context, repos []string) ([]gql.RepoCount, error)

	addPRComment       func(repo string, number int, body string) error
	addIssueComment    func(repo string, number int, body string) error
	closePR            func(repo string, number int) error
	reopenPR           func(repo string, number int) error
	closeIssue         func(repo string, number int) error
	reopenIssue        func(repo string, number int) error
	editPRLabels       func(repo string, number int, add, remove []string) error
	editIssueLabels    func(repo string, number int, add, remove []string) error
	editPRAssignees    func(repo string, number int, add, remove []string) error
	editIssueAssignees func(repo string, number int, add, remove []string) error
}

func (f fakeBackend) GetPR(ctx context.Context, repo string, number int) (gql.PullRequest, error) {
	return f.getPR(ctx, repo, number)
}

func (f fakeBackend) GetIssue(ctx context.Context, repo string, number int) (gql.Issue, error) {
	return f.getIssue(ctx, repo, number)
}

func (f fakeBackend) AddPRComment(repo string, number int, body string) error {
	return f.addPRComment(repo, number, body)
}

func (f fakeBackend) AddIssueComment(repo string, number int, body string) error {
	return f.addIssueComment(repo, number, body)
}

func (f fakeBackend) ClosePR(repo string, number int) error {
	return f.closePR(repo, number)
}

func (f fakeBackend) ReopenPR(repo string, number int) error {
	return f.reopenPR(repo, number)
}

func (f fakeBackend) CloseIssue(repo string, number int) error {
	return f.closeIssue(repo, number)
}

func (f fakeBackend) ReopenIssue(repo string, number int) error {
	return f.reopenIssue(repo, number)
}

func (f fakeBackend) EditPRLabels(repo string, number int, add, remove []string) error {
	return f.editPRLabels(repo, number, add, remove)
}

func (f fakeBackend) EditIssueLabels(repo string, number int, add, remove []string) error {
	return f.editIssueLabels(repo, number, add, remove)
}

func (f fakeBackend) EditPRAssignees(repo string, number int, add, remove []string) error {
	return f.editPRAssignees(repo, number, add, remove)
}

func (f fakeBackend) EditIssueAssignees(repo string, number int, add, remove []string) error {
	return f.editIssueAssignees(repo, number, add, remove)
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

// TestGetItemTranslatesTheWireShapeIntoTheDomain gives every field of
// gql.PullRequest and of gql.Issue a distinct, non-zero value and compares
// the whole resulting domain.Item against a fully written-out expectation,
// for the same reason as TestGetPRTranslatesTheWireShapeIntoTheDomain above:
// a field toItemFromPR or toItemFromIssue forgot to copy is left at its zero
// value, which a struct-wide comparison catches and a handful of field
// assertions would not. The issue case's struct-wide comparison also covers
// Change staying nil.
func TestGetItemTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	t.Run("pull request", func(t *testing.T) {
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
			BodyText:       "this adds the gateway (text)",
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

		item, err := g.GetItem(context.Background(), domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 59})
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}

		want := domain.Item{
			Ref:      domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 59},
			Title:    "add the gateway",
			Author:   domain.Author{Login: "kukv"},
			State:    domain.StateOpen,
			URL:      "https://github.com/kukv/octoscope/pull/59",
			Body:     "this adds the gateway",
			BodyText: "this adds the gateway (text)",
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
			UpdatedAt: updatedAt,
			Change: &domain.Change{
				IsDraft:   true,
				Review:    domain.ReviewApproved,
				Head:      "refactor/pr2b-gateway",
				Base:      "main",
				Additions: 42,
				Deletions: 7,
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
			},
		}
		if !reflect.DeepEqual(item, want) {
			t.Errorf("GetItem() = %+v, want %+v", item, want)
		}
	})

	t.Run("issue", func(t *testing.T) {
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
			BodyText:  "conversion tests only check a handful of fields (text)",
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

		item, err := g.GetItem(context.Background(), domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 54})
		if err != nil {
			t.Fatalf("GetItem: %v", err)
		}

		want := domain.Item{
			Ref:      domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 54},
			Title:    "track the conversion",
			Author:   domain.Author{Login: "kukv"},
			State:    domain.StateClosed,
			URL:      "https://github.com/kukv/octoscope/issues/54",
			Body:     "conversion tests only check a handful of fields",
			BodyText: "conversion tests only check a handful of fields (text)",
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
			UpdatedAt: updatedAt,
			Change:    nil,
		}
		if !reflect.DeepEqual(item, want) {
			t.Errorf("GetItem() = %+v, want %+v", item, want)
		}
	})
}

func TestParseItemState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state string
		want  domain.ItemState
	}{
		{"OPEN", domain.StateOpen},
		{"CLOSED", domain.StateClosed},
		{"MERGED", domain.StateMerged},
		// gh's REST output lower-cases what GraphQL sends in capitals.
		{"open", domain.StateOpen},
		{"merged", domain.StateMerged},
		{"", domain.StateClosed},
		{"SOMETHING_NEW", domain.StateClosed},
	}

	for _, tt := range tests {
		if got := parseItemState(tt.state); got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestParseReviewDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		decision string
		want     domain.ReviewState
	}{
		{"APPROVED", domain.ReviewApproved},
		{"CHANGES_REQUESTED", domain.ReviewChangesRequested},
		{"REVIEW_REQUIRED", domain.ReviewRequired},
		{"", domain.ReviewNone},
		{"SOMETHING_NEW", domain.ReviewNone},
	}

	for _, tt := range tests {
		if got := parseReviewDecision(tt.decision); got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.decision, got, tt.want)
		}
	}
}

// TestGetItemKeepsChangeAndKindInStep is the invariant domain.Item's doc
// states: Change is non-nil exactly when the reference says ItemPR. The
// gateway is what guarantees it, because the gateway is what builds an Item.
func TestGetItemKeepsChangeAndKindInStep(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ref        domain.ItemRef
		backend    fakeBackend
		wantKind   domain.ItemKind
		wantChange bool
	}{
		{
			name: "a pull request has a change",
			ref:  domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 7},
			backend: fakeBackend{getPR: func(context.Context, string, int) (gql.PullRequest, error) {
				return gql.PullRequest{Number: 7, HeadRefName: "topic", BaseRefName: "main"}, nil
			}},
			wantKind:   domain.ItemPR,
			wantChange: true,
		},
		{
			name: "an issue has none",
			ref:  domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 9},
			backend: fakeBackend{getIssue: func(context.Context, string, int) (gql.Issue, error) {
				return gql.Issue{Number: 9}, nil
			}},
			wantKind:   domain.ItemIssue,
			wantChange: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := New(tt.backend).GetItem(context.Background(), tt.ref)
			if err != nil {
				t.Fatalf("GetItem: %v", err)
			}
			if got.Ref.Kind != tt.wantKind {
				t.Errorf("Ref.Kind = %v, want %v", got.Ref.Kind, tt.wantKind)
			}
			if (got.Change != nil) != tt.wantChange {
				t.Errorf("Change != nil = %v, want %v", got.Change != nil, tt.wantChange)
			}
			if got.Ref != tt.ref {
				t.Errorf("Ref = %+v, want %+v", got.Ref, tt.ref)
			}
		})
	}
}
