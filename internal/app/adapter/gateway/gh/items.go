package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// GetPR returns one pull request with its body and conversation.
func (g *Gateway) GetPR(ctx context.Context, repo string, number int) (domain.PR, error) {
	n, err := g.backend.GetPR(ctx, repo, number)
	if err != nil {
		return domain.PR{}, err
	}
	return toPR(n), nil
}

// GetIssue returns one issue with its body and conversation.
func (g *Gateway) GetIssue(ctx context.Context, repo string, number int) (domain.Issue, error) {
	n, err := g.backend.GetIssue(ctx, repo, number)
	if err != nil {
		return domain.Issue{}, err
	}
	return toIssue(n), nil
}

func toPR(n gql.PullRequest) domain.PR {
	return domain.PR{
		Number:    n.Number,
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     domain.ParseItemState(n.State),
		IsDraft:   n.IsDraft,
		UpdatedAt: n.UpdatedAt,
		Review:    domain.ParseReviewDecision(n.ReviewDecision),
		URL:       n.URL,
		Body:      n.Body,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    toLabels(n.Labels.Nodes),
		Assignees: toAuthors(n.Assignees.Nodes),
		Checks:    toChecksFromContexts(n.StatusCheckContexts()),
		Head:      n.HeadRefName,
		Base:      n.BaseRefName,
		Additions: n.Additions,
		Deletions: n.Deletions,
	}
}

func toIssue(n gql.Issue) domain.Issue {
	return domain.Issue{
		Number:    n.Number,
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     domain.ParseItemState(n.State),
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
		Body:      n.Body,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    toLabels(n.Labels.Nodes),
		Assignees: toAuthors(n.Assignees.Nodes),
	}
}

func toComments(in []gql.Comment) []domain.Comment {
	if len(in) == 0 {
		return nil
	}
	out := make([]domain.Comment, len(in))
	for i, c := range in {
		out[i] = domain.Comment{
			Author:    domain.Author{Login: c.Author.Login},
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
		}
	}
	return out
}
