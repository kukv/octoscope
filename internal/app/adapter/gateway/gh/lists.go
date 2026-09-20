package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// ListPRs returns the open pull requests of one repository.
func (g *Gateway) ListPRs(ctx context.Context, repo string) ([]domain.PR, error) {
	nodes, err := g.backend.ListPRs(ctx, repo)
	if err != nil {
		return nil, wrap(err)
	}
	prs := make([]domain.PR, len(nodes))
	for i, n := range nodes {
		prs[i] = toPR(n)
	}
	return prs, nil
}

// ListIssues returns the open issues of one repository.
func (g *Gateway) ListIssues(ctx context.Context, repo string) ([]domain.Issue, error) {
	nodes, err := g.backend.ListIssues(ctx, repo)
	if err != nil {
		return nil, wrap(err)
	}
	issues := make([]domain.Issue, len(nodes))
	for i, n := range nodes {
		issues[i] = toIssue(n)
	}
	return issues, nil
}

// ListItems returns the open pull requests or the open issues of one
// repository. kind is which listing to run, not a branch in the caller: the
// Repos tab draws the two in separate panes and knows which it is filling.
func (g *Gateway) ListItems(ctx context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error) {
	if kind == domain.ItemPR {
		nodes, err := g.backend.ListPRs(ctx, repo)
		if err != nil {
			return nil, wrap(err)
		}
		items := make([]domain.Item, len(nodes))
		for i, n := range nodes {
			items[i] = toItemFromPR(n, repo)
		}
		return items, nil
	}
	nodes, err := g.backend.ListIssues(ctx, repo)
	if err != nil {
		return nil, wrap(err)
	}
	items := make([]domain.Item, len(nodes))
	for i, n := range nodes {
		items[i] = toItemFromIssue(n, repo)
	}
	return items, nil
}

// ListLabels names the repository's labels.
func (g *Gateway) ListLabels(ctx context.Context, repo string) ([]domain.Label, error) {
	labels, err := g.backend.ListLabels(ctx, repo)
	if err != nil {
		return nil, wrap(err)
	}
	return toLabels(labels), nil
}

func toLabels(in []gql.Label) []domain.Label {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Label, len(in))
	for i, l := range in {
		out[i] = domain.Label{Name: l.Name, Color: l.Color}
	}
	return out
}

func toAuthors(in []gql.Author) []domain.Author {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Author, len(in))
	for i, a := range in {
		out[i] = domain.Author{Login: a.Login}
	}
	return out
}
