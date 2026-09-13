package gh

import (
	"context"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// fakeBackend answers whichever method a test set a function for. Embedding
// the nil backend interface, rather than stubbing every method by hand,
// panics loudly if a test calls a method it did not mean to exercise.
type fakeBackend struct {
	backend
	getPR    func(ctx context.Context, repo string, number int) (gql.PullRequest, error)
	getIssue func(ctx context.Context, repo string, number int) (gql.Issue, error)
}

func (f fakeBackend) GetPR(ctx context.Context, repo string, number int) (gql.PullRequest, error) {
	return f.getPR(ctx, repo, number)
}

func (f fakeBackend) GetIssue(ctx context.Context, repo string, number int) (gql.Issue, error) {
	return f.getIssue(ctx, repo, number)
}

func TestGetPRTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	node := gql.PullRequest{
		Number:         59,
		Title:          "add the gateway",
		State:          "OPEN",
		ReviewDecision: "APPROVED",
		UpdatedAt:      when,
		Author:         gql.Author{Login: "kukv"},
	}
	node.Labels.Nodes = []gql.Label{{Name: "bug", Color: "d73a4a"}}
	node.Assignees.Nodes = []gql.Author{{Login: "octocat"}}
	node.Comments.Nodes = []gql.Comment{{Author: gql.Author{Login: "octocat"}, Body: "lgtm", CreatedAt: when}}

	g := New(fakeBackend{getPR: func(context.Context, string, int) (gql.PullRequest, error) {
		return node, nil
	}})

	pr, err := g.GetPR(context.Background(), "kukv/octoscope", 59)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.State != domain.StateOpen {
		t.Errorf("state = %v, want StateOpen", pr.State)
	}
	// This is the assertion the destruction check exercises: a conversion
	// that stops calling domain.ParseReviewDecision still builds a domain.PR,
	// just with the zero ReviewState, and nothing else here would notice.
	if pr.Review != domain.ReviewApproved {
		t.Errorf("review = %v, want ReviewApproved", pr.Review)
	}
	if len(pr.Labels) != 1 || pr.Labels[0].Name != "bug" || pr.Labels[0].Color != "d73a4a" {
		t.Errorf("labels = %+v, want one bug/d73a4a label", pr.Labels)
	}
	if len(pr.Assignees) != 1 || pr.Assignees[0].Login != "octocat" {
		t.Errorf("assignees = %+v, want one octocat", pr.Assignees)
	}
	if len(pr.Comments) != 1 || pr.Comments[0].Body != "lgtm" {
		t.Errorf("comments = %+v, want one lgtm", pr.Comments)
	}
}

func TestGetIssueTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)
	node := gql.Issue{
		Number:    54,
		Title:     "track the conversion",
		State:     "CLOSED",
		UpdatedAt: when,
		Author:    gql.Author{Login: "kukv"},
	}
	node.Labels.Nodes = []gql.Label{{Name: "docs", Color: "0075ca"}}
	node.Assignees.Nodes = []gql.Author{{Login: "octocat"}}
	node.Comments.Nodes = []gql.Comment{{Author: gql.Author{Login: "octocat"}, Body: "done", CreatedAt: when}}

	g := New(fakeBackend{getIssue: func(context.Context, string, int) (gql.Issue, error) {
		return node, nil
	}})

	issue, err := g.GetIssue(context.Background(), "kukv/octoscope", 54)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.State != domain.StateClosed {
		t.Errorf("state = %v, want StateClosed", issue.State)
	}
	if len(issue.Labels) != 1 || issue.Labels[0].Name != "docs" {
		t.Errorf("labels = %+v, want one docs label", issue.Labels)
	}
	if len(issue.Assignees) != 1 || issue.Assignees[0].Login != "octocat" {
		t.Errorf("assignees = %+v, want one octocat", issue.Assignees)
	}
	if len(issue.Comments) != 1 || issue.Comments[0].Body != "done" {
		t.Errorf("comments = %+v, want one done", issue.Comments)
	}
}
