package gh

import (
	"context"
	"testing"

	"github.com/kukv/octoscope/internal/github/gql"
)

// fakeLister answers ListPRs, ListIssues and ListLabels with whatever a test
// set. Embedding the nil backend panics loudly if a test calls a method it
// did not mean to exercise.
type fakeLister struct {
	backend
	listPRs    func(ctx context.Context, repo string) ([]gql.PullRequest, error)
	listIssues func(ctx context.Context, repo string) ([]gql.Issue, error)
	listLabels func(ctx context.Context, repo string) ([]gql.Label, error)
}

func (f fakeLister) ListPRs(ctx context.Context, repo string) ([]gql.PullRequest, error) {
	return f.listPRs(ctx, repo)
}

func (f fakeLister) ListIssues(ctx context.Context, repo string) ([]gql.Issue, error) {
	return f.listIssues(ctx, repo)
}

func (f fakeLister) ListLabels(ctx context.Context, repo string) ([]gql.Label, error) {
	return f.listLabels(ctx, repo)
}

func TestListPRsTranslatesEveryNode(t *testing.T) {
	t.Parallel()

	g := New(fakeLister{listPRs: func(context.Context, string) ([]gql.PullRequest, error) {
		return []gql.PullRequest{
			{Number: 1, State: "OPEN"},
			{Number: 2, State: "MERGED"},
		}, nil
	}})

	prs, err := g.ListPRs(context.Background(), "kukv/octoscope")
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) != 2 || prs[0].Number != 1 || prs[1].Number != 2 {
		t.Errorf("prs = %+v, want numbers 1 and 2 in order", prs)
	}
}

func TestListIssuesTranslatesEveryNode(t *testing.T) {
	t.Parallel()

	g := New(fakeLister{listIssues: func(context.Context, string) ([]gql.Issue, error) {
		return []gql.Issue{{Number: 3}, {Number: 4}}, nil
	}})

	issues, err := g.ListIssues(context.Background(), "kukv/octoscope")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 2 || issues[0].Number != 3 || issues[1].Number != 4 {
		t.Errorf("issues = %+v, want numbers 3 and 4 in order", issues)
	}
}

func TestListLabelsTranslatesEveryLabel(t *testing.T) {
	t.Parallel()

	g := New(fakeLister{listLabels: func(context.Context, string) ([]gql.Label, error) {
		return []gql.Label{{Name: "bug", Color: "d73a4a"}, {Name: "wip", Color: "ededed"}}, nil
	}})

	labels, err := g.ListLabels(context.Background(), "kukv/octoscope")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 2 || labels[0].Name != "bug" || labels[0].Color != "d73a4a" || labels[1].Name != "wip" {
		t.Errorf("labels = %+v, want bug/d73a4a and wip", labels)
	}
}
