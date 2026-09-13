package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// PRMergeContext fetches what the merge popup draws: what the repository
// allows and what state this pull request is in.
func (g *Gateway) PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error) {
	c, err := g.backend.PRMergeContext(ctx, repo, number)
	if err != nil {
		return domain.MergeContext{}, err
	}
	return toMergeContext(c), nil
}

// MergePR merges the pull request now.
func (g *Gateway) MergePR(pullRequestID string, method domain.MergeMethod) error {
	return g.backend.MergePR(pullRequestID, fromMergeMethod(method))
}

// EnableAutoMerge asks GitHub to merge the pull request once what it is
// waiting on is in.
func (g *Gateway) EnableAutoMerge(pullRequestID string, method domain.MergeMethod) error {
	return g.backend.EnableAutoMerge(pullRequestID, fromMergeMethod(method))
}

func toMergeContext(c gql.MergeContext) domain.MergeContext {
	return domain.MergeContext{
		PullRequestID:            c.PullRequestID,
		IsDraft:                  c.IsDraft,
		Mergeable:                parseMergeable(c.Mergeable),
		State:                    parseMergeState(c.MergeStateStatus),
		Review:                   domain.ParseReviewDecision(c.ReviewDecision),
		Methods:                  allowedMethods(c.SquashMergeAllowed, c.MergeCommitAllowed, c.RebaseMergeAllowed),
		DeleteBranchOnMerge:      c.DeleteBranchOnMerge,
		AutoMergeAllowed:         c.AutoMergeAllowed,
		ViewerCanEnableAutoMerge: c.ViewerCanEnableAutoMerge,
		AutoMergeEnabled:         c.AutoMergeEnabled,
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

// A value neither of these knows is read as "unknown" rather than failing
// the fetch: GitHub adds values to these enums.
func parseMergeable(s string) domain.Mergeable {
	switch s {
	case "MERGEABLE":
		return domain.MergeableYes
	case "CONFLICTING":
		return domain.MergeableConflicting
	}
	return domain.MergeableUnknown
}

func parseMergeState(s string) domain.MergeState {
	switch s {
	case "CLEAN":
		return domain.MergeStateClean
	case "BLOCKED":
		return domain.MergeStateBlocked
	case "BEHIND":
		return domain.MergeStateBehind
	case "DIRTY":
		return domain.MergeStateDirty
	case "UNSTABLE":
		return domain.MergeStateUnstable
	case "HAS_HOOKS":
		return domain.MergeStateHasHooks
	}
	return domain.MergeStateUnknown
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
