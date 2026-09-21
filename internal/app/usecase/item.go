package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

// itemFetcher is one item, whichever of the two the reference names. Which of
// GitHub's two APIs answers is the gateway's business.
type itemFetcher interface {
	GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error)
}

type commenter interface {
	AddComment(ctx context.Context, ref domain.ItemRef, body string) error
}

type stateChanger interface {
	SetState(ctx context.Context, ref domain.ItemRef, closing bool) error
}

type labelEditor interface {
	EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error
}

type assigneeEditor interface {
	EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error
}

func (u *Usecase) GetItem(ctx context.Context, ref domain.ItemRef) (domain.Item, error) {
	return u.items.GetItem(ctx, ref)
}

func (u *Usecase) AddComment(ctx context.Context, ref domain.ItemRef, body string) error {
	return u.comments.AddComment(ctx, ref, body)
}

// SetState closes the item when closing is true and reopens it otherwise.
func (u *Usecase) SetState(ctx context.Context, ref domain.ItemRef, closing bool) error {
	return u.states.SetState(ctx, ref, closing)
}

func (u *Usecase) EditLabels(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	return u.labels.EditLabels(ctx, ref, add, remove)
}

func (u *Usecase) EditAssignees(ctx context.Context, ref domain.ItemRef, add, remove []string) error {
	return u.assignees.EditAssignees(ctx, ref, add, remove)
}
