package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// The four operations below are where the difference between a pull request
// and an issue stops. GitHub gives each of them two endpoints; the
// application has one operation, and this layer is what joins them.
//
// ctx is taken and not used: the client's write methods do not accept one
// yet. The port takes it so that threading it through later changes this
// file and not every caller.

// AddComment posts one comment on the item the reference names.
func (g *Gateway) AddComment(ctx context.Context, ref domain.ItemRef, body string) error {
	_ = ctx
	if ref.Kind == domain.ItemPR {
		return wrap(g.AddPRComment(ref.Repo, ref.Number, body))
	}
	return wrap(g.AddIssueComment(ref.Repo, ref.Number, body))
}

// SetState closes the item when closing is true and reopens it otherwise.
func (g *Gateway) SetState(ctx context.Context, ref domain.ItemRef, closing bool) error {
	_ = ctx
	switch {
	case ref.Kind == domain.ItemPR && closing:
		return wrap(g.ClosePR(ref.Repo, ref.Number))
	case ref.Kind == domain.ItemPR:
		return wrap(g.ReopenPR(ref.Repo, ref.Number))
	case closing:
		return wrap(g.CloseIssue(ref.Repo, ref.Number))
	default:
		return wrap(g.ReopenIssue(ref.Repo, ref.Number))
	}
}

// EditLabels adds and removes labels in one call.
func (g *Gateway) EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	_ = ctx
	if ref.Kind == domain.ItemPR {
		return wrap(g.EditPRLabels(ref.Repo, ref.Number, add, remove))
	}
	return wrap(g.EditIssueLabels(ref.Repo, ref.Number, add, remove))
}

// EditAssignees adds and removes assignees in one call.
func (g *Gateway) EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	_ = ctx
	if ref.Kind == domain.ItemPR {
		return wrap(g.EditPRAssignees(ref.Repo, ref.Number, add, remove))
	}
	return wrap(g.EditIssueAssignees(ref.Repo, ref.Number, add, remove))
}
