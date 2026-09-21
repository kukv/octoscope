package gh

import (
	"context"
	"fmt"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// workQueries is each column's GitHub search. The strings are fixed text,
// not user input: they are the definition of what the column means.
//
// archived:false is on every column because an archived repository is
// read-only: its pull requests cannot be merged or reviewed and its issues
// cannot be closed, so a card from one is work nobody can pick up.
var workQueries = [domain.WorkSectionCount]string{
	domain.SectionReviewRequested: "is:open is:pr review-requested:@me archived:false",
	domain.SectionYourPRs:         "is:open is:pr author:@me archived:false",
	domain.SectionAssigned:        "is:open assignee:@me archived:false",
	domain.SectionMentioned:       "is:open mentions:@me archived:false",
}

func workQuery(s domain.WorkSection) string {
	return workQueries[s]
}

// ListWorkSection fetches one column of the Work board. Its search string is
// fixed text, not anything the user typed.
func (g *Gateway) ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error) {
	if s < 0 || int(s) >= len(workQueries) {
		return nil, fmt.Errorf("unknown work section %d", s)
	}
	nodes, err := g.backend.SearchItems(ctx, workQuery(s))
	if err != nil {
		return nil, wrap(err)
	}
	return toWorkItems(nodes), nil
}

func (g *Gateway) SearchItems(ctx context.Context, query string) ([]domain.WorkItem, error) {
	nodes, err := g.backend.SearchItems(ctx, query)
	if err != nil {
		return nil, wrap(err)
	}
	return toWorkItems(nodes), nil
}

func toWorkItems(nodes []gql.SearchItem) []domain.WorkItem {
	items := make([]domain.WorkItem, len(nodes))
	for i, n := range nodes {
		items[i] = toWorkItem(n)
	}
	return items
}

func toWorkItem(n gql.SearchItem) domain.WorkItem {
	item := domain.WorkItem{
		Ref: domain.ItemRef{
			Kind:   domain.ItemIssue,
			Repo:   n.Repository.NameWithOwner,
			Number: n.Number,
		},
		Title:     n.Title,
		State:     parseItemState(n.State),
		Body:      n.BodyText,
		Author:    n.Author.Login,
		Labels:    toLabels(n.Labels.Nodes),
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
	}
	if n.Typename != "PullRequest" {
		return item
	}
	item.Ref.Kind = domain.ItemPR
	item.IsDraft = n.IsDraft
	item.Review = parseReviewDecision(n.ReviewDecision)
	item.Head = n.HeadRefName
	item.Base = n.BaseRefName
	item.Additions = n.Additions
	item.Deletions = n.Deletions
	item.Checks = toChecksFromContexts(n.StatusCheckContexts())
	return item
}

func (g *Gateway) RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error) {
	nodes, err := g.backend.RepoCounts(ctx, repos)
	if err != nil {
		return nil, wrap(err)
	}
	counts := make([]domain.RepoCount, len(nodes))
	for i, n := range nodes {
		counts[i] = toRepoCount(n)
	}
	return counts, nil
}

func toRepoCount(n gql.RepoCount) domain.RepoCount {
	return domain.RepoCount{
		Repo:        n.Repo,
		PRs:         n.PRs,
		Issues:      n.Issues,
		Unavailable: n.Unavailable,
	}
}
