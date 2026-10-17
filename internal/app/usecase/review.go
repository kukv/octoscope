package usecase

import (
	"context"
	"fmt"

	"github.com/kukv/octoscope/internal/app/domain"
)

type reviewFetcher interface {
	PRDiff(ctx context.Context, repo string, number int) ([]domain.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error)
}

type reviewer interface {
	AddReviewThread(ctx context.Context, t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, domain.ReviewThread, error)
	SubmitReview(ctx context.Context, t domain.ReviewTarget, event domain.ReviewEvent, body string) error
	DiscardReview(ctx context.Context, review domain.ReviewHandle) error
}

func (u *Usecase) PRDiff(ctx context.Context, repo string, number int) ([]domain.FileDiff, error) {
	return u.reviewInfo.PRDiff(ctx, repo, number)
}

func (u *Usecase) PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error) {
	return u.reviewInfo.PRReviewContext(ctx, repo, number)
}

func (u *Usecase) DiscardReview(ctx context.Context, review domain.ReviewHandle) error {
	return u.reviews.DiscardReview(ctx, review)
}

// PostLineComment attaches one line comment to the pull request's review and
// answers the review it went onto and the thread it created: which requests
// that takes is the gateway's knowledge, not the application's.
func (u *Usecase) PostLineComment(ctx context.Context, t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, domain.ReviewThread, error) {
	id, thread, err := u.reviews.AddReviewThread(ctx, t, c)
	if err != nil {
		return "", domain.ReviewThread{}, fmt.Errorf("add review thread: %w", err)
	}
	return id, thread, nil
}

func (u *Usecase) SubmitReview(ctx context.Context, t domain.ReviewTarget, event domain.ReviewEvent, body string) error {
	if err := u.reviews.SubmitReview(ctx, t, event, body); err != nil {
		return fmt.Errorf("submit review: %w", err)
	}
	return nil
}
