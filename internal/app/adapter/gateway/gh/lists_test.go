package gh

import (
	"context"
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
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

// list asks for bodyText and not the markdown body, so the two travel in
// fields of their own: a conversion that dropped this one would leave the
// preview empty with nothing to notice.
func TestAListedItemCarriesItsBodyAsText(t *testing.T) {
	t.Parallel()

	g := New(fakeLister{
		listPRs: func(context.Context, string) ([]gql.PullRequest, error) {
			return []gql.PullRequest{{Number: 1, BodyText: "what it changes"}}, nil
		},
		listIssues: func(context.Context, string) ([]gql.Issue, error) {
			return []gql.Issue{{Number: 2, BodyText: "what is wrong"}}, nil
		},
	})

	prs, err := g.ListItems(context.Background(), "kukv/octoscope", domain.ItemPR)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if prs[0].BodyText != "what it changes" {
		t.Errorf("the pull request's body text is %q", prs[0].BodyText)
	}

	issues, err := g.ListItems(context.Background(), "kukv/octoscope", domain.ItemIssue)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if issues[0].BodyText != "what is wrong" {
		t.Errorf("the issue's body text is %q", issues[0].BodyText)
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
	want := []domain.Label{
		{Name: "bug", Color: "d73a4a"},
		{Name: "wip", Color: "ededed"},
	}
	if !reflect.DeepEqual(labels, want) {
		t.Errorf("ListLabels() = %+v, want %+v", labels, want)
	}
}

// TestListItemsPicksTheQueryByKind checks the other half of the dispatch:
// which of GitHub's two listings runs is decided here, not above.
func TestListItemsPicksTheQueryByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull requests", func(t *testing.T) {
		t.Parallel()
		b := fakeBackend{listPRs: func(context.Context, string) ([]gql.PullRequest, error) {
			return []gql.PullRequest{{Number: 1}, {Number: 2}}, nil
		}}
		got, err := New(b).ListItems(context.Background(), "kukv/octoscope", domain.ItemPR)
		if err != nil {
			t.Fatalf("ListItems: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d items, want 2", len(got))
		}
		for _, item := range got {
			if item.Ref.Kind != domain.ItemPR || item.Change == nil {
				t.Errorf("item %+v is not a pull request with a change", item.Ref)
			}
			if item.Ref.Repo != "kukv/octoscope" {
				t.Errorf("Ref.Repo = %q, want the repository asked for", item.Ref.Repo)
			}
		}
	})
	t.Run("issues", func(t *testing.T) {
		t.Parallel()
		b := fakeBackend{listIssues: func(context.Context, string) ([]gql.Issue, error) {
			return []gql.Issue{{Number: 3}}, nil
		}}
		got, err := New(b).ListItems(context.Background(), "kukv/octoscope", domain.ItemIssue)
		if err != nil {
			t.Fatalf("ListItems: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d items, want 1", len(got))
		}
		if got[0].Ref.Kind != domain.ItemIssue || got[0].Change != nil {
			t.Errorf("item %+v is not an issue without a change", got[0].Ref)
		}
	})
}
