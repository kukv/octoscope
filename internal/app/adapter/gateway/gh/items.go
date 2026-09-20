package gh

import (
	"context"
	"strings"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// GetPR returns one pull request with its body and conversation.
func (g *Gateway) GetPR(ctx context.Context, repo string, number int) (domain.PR, error) {
	n, err := g.backend.GetPR(ctx, repo, number)
	if err != nil {
		return domain.PR{}, wrap(err)
	}
	return toPR(n), nil
}

// GetIssue returns one issue with its body and conversation.
func (g *Gateway) GetIssue(ctx context.Context, repo string, number int) (domain.Issue, error) {
	n, err := g.backend.GetIssue(ctx, repo, number)
	if err != nil {
		return domain.Issue{}, wrap(err)
	}
	return toIssue(n), nil
}

// GetItem returns whichever of the two the reference names, with its body and
// conversation. Which of GitHub's two queries that takes is this layer's
// knowledge: nothing above it switches on the kind.
func (g *Gateway) GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error) {
	if ref.Kind == domain.ItemPR {
		return g.getPRItem(ctx, ref.Repo, ref.Number)
	}
	return g.getIssueItem(ctx, ref.Repo, ref.Number)
}

func (g *Gateway) getPRItem(ctx context.Context, repo string, number int) (domain.Item, error) {
	n, err := g.backend.GetPR(ctx, repo, number)
	if err != nil {
		return domain.Item{}, wrap(err)
	}
	return toItemFromPR(n, repo), nil
}

func (g *Gateway) getIssueItem(ctx context.Context, repo string, number int) (domain.Item, error) {
	n, err := g.backend.GetIssue(ctx, repo, number)
	if err != nil {
		return domain.Item{}, wrap(err)
	}
	return toItemFromIssue(n, repo), nil
}

func toPR(n gql.PullRequest) domain.PR {
	return domain.PR{
		Number:    n.Number,
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     parseItemState(n.State),
		IsDraft:   n.IsDraft,
		UpdatedAt: n.UpdatedAt,
		Review:    parseReviewDecision(n.ReviewDecision),
		URL:       n.URL,
		Body:      n.Body,
		BodyText:  n.BodyText,
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
		State:     parseItemState(n.State),
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
		Body:      n.Body,
		BodyText:  n.BodyText,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    toLabels(n.Labels.Nodes),
		Assignees: toAuthors(n.Assignees.Nodes),
	}
}

// toItemFromPR builds the domain's Item out of a pull request. Change is
// always set here: that is the half of domain.Item's invariant this function
// owns, and toItemFromIssue owns the other.
func toItemFromPR(n gql.PullRequest, repo string) domain.Item {
	return domain.Item{
		Ref:       domain.ItemRef{Kind: domain.ItemPR, Repo: repo, Number: n.Number},
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     parseItemState(n.State),
		URL:       n.URL,
		Body:      n.Body,
		BodyText:  n.BodyText,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    toLabels(n.Labels.Nodes),
		Assignees: toAuthors(n.Assignees.Nodes),
		UpdatedAt: n.UpdatedAt,
		Change: &domain.Change{
			IsDraft:   n.IsDraft,
			Review:    parseReviewDecision(n.ReviewDecision),
			Head:      n.HeadRefName,
			Base:      n.BaseRefName,
			Additions: n.Additions,
			Deletions: n.Deletions,
			Checks:    toChecksFromContexts(n.StatusCheckContexts()),
		},
	}
}

// toItemFromIssue builds the domain's Item out of an issue. Change stays nil:
// an issue proposes no change.
func toItemFromIssue(n gql.Issue, repo string) domain.Item {
	return domain.Item{
		Ref:       domain.ItemRef{Kind: domain.ItemIssue, Repo: repo, Number: n.Number},
		Title:     n.Title,
		Author:    domain.Author{Login: n.Author.Login},
		State:     parseItemState(n.State),
		URL:       n.URL,
		Body:      n.Body,
		BodyText:  n.BodyText,
		Comments:  toComments(n.Comments.Nodes),
		Labels:    toLabels(n.Labels.Nodes),
		Assignees: toAuthors(n.Assignees.Nodes),
		UpdatedAt: n.UpdatedAt,
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

// parseItemState maps GitHub's state onto the domain value. GraphQL and REST
// differ in case, so the comparison ignores it; anything unrecognised reads
// as closed, which is the reading that offers no action.
func parseItemState(state string) domain.ItemState {
	switch strings.ToUpper(state) {
	case "OPEN":
		return domain.StateOpen
	case "MERGED":
		return domain.StateMerged
	default:
		return domain.StateClosed
	}
}

// parseReviewDecision maps the GraphQL reviewDecision enum onto the domain
// value. An empty string means the pull request needs no review at all; an
// unknown one is treated the same way rather than failing the whole fetch.
func parseReviewDecision(decision string) domain.ReviewState {
	switch decision {
	case "APPROVED":
		return domain.ReviewApproved
	case "CHANGES_REQUESTED":
		return domain.ReviewChangesRequested
	case "REVIEW_REQUIRED":
		return domain.ReviewRequired
	default:
		return domain.ReviewNone
	}
}
