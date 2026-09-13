package usecase

import (
	"fmt"

	"github.com/kukv/octoscope/internal/app/domain"
)

// PostLineComment attaches one line comment to the pull request's unsubmitted
// review, starting that review first if there is none: on GitHub a line
// comment has to hang off a review.
func (u *Usecase) PostLineComment(t domain.ReviewTarget, c domain.PendingComment) (domain.ReviewHandle, error) {
	review := t.Pending
	if review == "" {
		r, err := u.reviews.StartReview(t.PullRequest)
		if err != nil {
			return "", fmt.Errorf("start review: %w", err)
		}
		review = r
	}
	if err := u.reviews.AddReviewThread(review, c); err != nil {
		return "", fmt.Errorf("add review thread: %w", err)
	}
	return review, nil
}

// SubmitReview sends the review out. With nothing waiting it creates and
// submits in one call: starting a review first would leave an empty pending
// review behind if the submission then failed.
func (u *Usecase) SubmitReview(t domain.ReviewTarget, event domain.ReviewEvent, body string) error {
	if t.Pending != "" {
		if err := u.reviews.SubmitReview(t.Pending, event, body); err != nil {
			return fmt.Errorf("submit review: %w", err)
		}
		return nil
	}
	if err := u.reviews.SubmitNewReview(t.PullRequest, event, body); err != nil {
		return fmt.Errorf("submit new review: %w", err)
	}
	return nil
}
