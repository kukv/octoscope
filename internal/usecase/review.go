package usecase

import (
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
)

// ReviewTarget names the pull request a review acts on, and the unsubmitted
// review already open on it if there is one. GitHub allows one unsubmitted
// review per user per pull request, so PendingID is a single id, not a list.
type ReviewTarget struct {
	PullRequestID string
	PendingID     string
}

// PostLineComment attaches one line comment to the pull request's unsubmitted
// review, starting that review first if there is none.
//
// The order is GitHub's, not the diff view's: a line comment has to hang off
// a review, and a review the reader never submits must not be created just
// because they opened the diff.
func (u *Usecase) PostLineComment(t ReviewTarget, c gh.PendingComment) (string, error) {
	reviewID := t.PendingID
	if reviewID == "" {
		id, err := u.reviews.StartReview(t.PullRequestID)
		if err != nil {
			return "", fmt.Errorf("start review: %w", err)
		}
		reviewID = id
	}
	if err := u.reviews.AddReviewThread(reviewID, c); err != nil {
		return "", fmt.Errorf("add review thread: %w", err)
	}
	return reviewID, nil
}

// SubmitReview sends the review out. When line comments are already waiting
// it submits the review they are on; when nothing is waiting it creates and
// submits in one call, so a plain approval leaves nothing behind if it fails.
func (u *Usecase) SubmitReview(t ReviewTarget, event gh.ReviewEvent, body string) error {
	if t.PendingID != "" {
		if err := u.reviews.SubmitReview(t.PendingID, event, body); err != nil {
			return fmt.Errorf("submit review: %w", err)
		}
		return nil
	}
	if err := u.reviews.SubmitNewReview(t.PullRequestID, event, body); err != nil {
		return fmt.Errorf("submit new review: %w", err)
	}
	return nil
}
