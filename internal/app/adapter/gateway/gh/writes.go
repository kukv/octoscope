package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// The four operations below are where the difference between a pull request
// and an issue stops. GitHub gives each of them two endpoints; the
// application has one operation, and this layer is what joins them.
//
// Each one picks its call first and wraps the failure afterwards, so the
// dispatch reads on its own and the translation happens once.

// AddComment posts one comment on the item the reference names.
func (g *Gateway) AddComment(ctx context.Context, ref domain.ItemRef, body string) error {
	var err error
	if ref.Kind == domain.ItemPR {
		err = g.backend.AddPRComment(ctx, ref.Repo, ref.Number, body)
	} else {
		err = g.backend.AddIssueComment(ctx, ref.Repo, ref.Number, body)
	}
	if err != nil {
		return wrap(err)
	}
	return nil
}

// SetState closes the item when closing is true and reopens it otherwise.
func (g *Gateway) SetState(ctx context.Context, ref domain.ItemRef, closing bool) error {
	var err error
	switch {
	case ref.Kind == domain.ItemPR && closing:
		err = g.backend.ClosePR(ctx, ref.Repo, ref.Number)
	case ref.Kind == domain.ItemPR:
		err = g.backend.ReopenPR(ctx, ref.Repo, ref.Number)
	case closing:
		err = g.backend.CloseIssue(ctx, ref.Repo, ref.Number)
	default:
		err = g.backend.ReopenIssue(ctx, ref.Repo, ref.Number)
	}
	if err != nil {
		return wrap(err)
	}
	return nil
}

// EditLabels adds and removes labels in one call.
func (g *Gateway) EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	var err error
	if ref.Kind == domain.ItemPR {
		err = g.backend.EditPRLabels(ctx, ref.Repo, ref.Number, add, remove)
	} else {
		err = g.backend.EditIssueLabels(ctx, ref.Repo, ref.Number, add, remove)
	}
	if err != nil {
		return wrap(err)
	}
	return nil
}

// EditAssignees adds and removes assignees in one call.
func (g *Gateway) EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	var err error
	if ref.Kind == domain.ItemPR {
		err = g.backend.EditPRAssignees(ctx, ref.Repo, ref.Number, add, remove)
	} else {
		err = g.backend.EditIssueAssignees(ctx, ref.Repo, ref.Number, add, remove)
	}
	if err != nil {
		return wrap(err)
	}
	return nil
}
