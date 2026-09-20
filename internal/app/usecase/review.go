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
	AddReviewThread(t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, error)
	SubmitReview(t domain.ReviewTarget, event domain.ReviewEvent, body string) error
	DiscardReview(review domain.ReviewHandle) error
}

func (u *Usecase) PRDiff(ctx context.Context, repo string, number int) ([]domain.FileDiff, error) {
	return u.reviewInfo.PRDiff(ctx, repo, number)
}

func (u *Usecase) PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error) {
	return u.reviewInfo.PRReviewContext(ctx, repo, number)
}

func (u *Usecase) DiscardReview(review domain.ReviewHandle) error {
	return u.reviews.DiscardReview(review)
}

// PostLineComment attaches one line comment to the pull request's review and
// answers the review it went onto: which requests that takes is the
// gateway's knowledge, not the application's.
func (u *Usecase) PostLineComment(t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, error) {
	id, err := u.reviews.AddReviewThread(t, c)
	if err != nil {
		return "", fmt.Errorf("add review thread: %w", err)
	}
	return id, nil
}

// SubmitReview sends the review out.
func (u *Usecase) SubmitReview(t domain.ReviewTarget, event domain.ReviewEvent, body string) error {
	if err := u.reviews.SubmitReview(t, event, body); err != nil {
		return fmt.Errorf("submit review: %w", err)
	}
	return nil
}
