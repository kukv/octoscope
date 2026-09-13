package usecase

import (
	"fmt"

	"github.com/kukv/octoscope/internal/app/domain"
)

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
