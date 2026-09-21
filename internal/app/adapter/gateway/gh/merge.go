package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

func (g *Gateway) PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error) {
	c, err := g.backend.PRMergeContext(ctx, repo, number)
	if err != nil {
		return domain.MergeContext{}, wrap(err)
	}
	return toMergeContext(c), nil
}

func (g *Gateway) MergePR(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error {
	if err := g.backend.MergePR(ctx, string(pr), fromMergeMethod(method)); err != nil {
		return wrap(err)
	}
	return nil
}

func (g *Gateway) EnableAutoMerge(ctx context.Context, pr domain.PullRequestHandle, method domain.MergeMethod) error {
	if err := g.backend.EnableAutoMerge(ctx, string(pr), fromMergeMethod(method)); err != nil {
		return wrap(err)
	}
	return nil
}

func (g *Gateway) DisableAutoMerge(ctx context.Context, pr domain.PullRequestHandle) error {
	if err := g.backend.DisableAutoMerge(ctx, string(pr)); err != nil {
		return wrap(err)
	}
	return nil
}

func toMergeContext(c gql.MergeContext) domain.MergeContext {
	return domain.MergeContext{
		PullRequest:              domain.PullRequestHandle(c.PullRequestID),
		Block:                    toMergeBlock(c.IsDraft, c.Mergeable, c.MergeStateStatus),
		Clean:                    c.MergeStateStatus == "CLEAN",
		Review:                   parseReviewDecision(c.ReviewDecision),
		Methods:                  allowedMethods(c.SquashMergeAllowed, c.MergeCommitAllowed, c.RebaseMergeAllowed),
		DeleteBranchOnMerge:      c.DeleteBranchOnMerge,
		AutoMergeAllowed:         c.AutoMergeAllowed,
		ViewerCanEnableAutoMerge: c.ViewerCanEnableAutoMerge,
		AutoMergeEnabled:         c.AutoMergeEnabled,
		// viewerCanMergeAsAdmin only reads classic branch protection, so it
		// answers false under a ruleset even for a viewer the ruleset lists
		// as an always-bypass actor. The repository permission is a
		// near-enough stand-in for "may bypass": a ruleset can name other
		// roles as bypass actors, and an admin need not be one of them.
		ViewerIsAdmin: c.ViewerPermission == "ADMIN",
	}
}

// allowedMethods lists the methods in the order the popup draws them
// (standalone design §4.4.4): squash, merge commit, rebase.
func allowedMethods(squash, commit, rebase bool) []domain.MergeMethod {
	var methods []domain.MergeMethod
	if squash {
		methods = append(methods, domain.MergeSquash)
	}
	if commit {
		methods = append(methods, domain.MergeCommit)
	}
	if rebase {
		methods = append(methods, domain.MergeRebase)
	}
	return methods
}

// toMergeBlock reads what the service reported into the one thing the
// application asks: why is this refused right now. Draft comes first
// because GitHub reports a draft as BLOCKED, and "it is a draft" is the
// more useful of the two.
//
// A spelling this does not know is not a failure -- GitHub adds values to
// these enums. An unknown mergeable means the answer is not worked out yet;
// an unknown state refuses nothing.
func toMergeBlock(isDraft bool, mergeable, state string) domain.MergeBlock {
	switch {
	case isDraft:
		return domain.BlockDraft
	case mergeable == "CONFLICTING":
		return domain.BlockConflicting
	case mergeable != "MERGEABLE":
		return domain.BlockComputing
	case state == "BLOCKED":
		return domain.BlockProtected
	case state == "BEHIND":
		return domain.BlockBehind
	case state == "DIRTY":
		return domain.BlockDirty
	}
	return domain.BlockNone
}

// fromMergeMethod spells a method the way the GraphQL
// PullRequestMergeMethod enum does. It is the one place that knows those
// words (.claude/rules/architecture.md).
func fromMergeMethod(m domain.MergeMethod) gql.MergeMethod {
	switch m {
	case domain.MergeCommit:
		return gql.MergeMethodMerge
	case domain.MergeRebase:
		return gql.MergeMethodRebase
	default:
		return gql.MergeMethodSquash
	}
}
