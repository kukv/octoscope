package usecase

import (
	"errors"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

type fakeReviewer struct {
	calls     []string
	newID     string
	startErr  error
	threadErr error
	submitErr error
}

func (f *fakeReviewer) StartReview(_ domain.PullRequestHandle) (domain.ReviewHandle, error) {
	f.calls = append(f.calls, "StartReview")
	return domain.ReviewHandle(f.newID), f.startErr
}

func (f *fakeReviewer) AddReviewThread(review domain.ReviewHandle, _ domain.PendingComment) error {
	f.calls = append(f.calls, "AddReviewThread("+string(review)+")")
	return f.threadErr
}

func (f *fakeReviewer) SubmitReview(_ domain.ReviewHandle, _ domain.ReviewEvent, _ string) error {
	f.calls = append(f.calls, "SubmitReview")
	return f.submitErr
}

func (f *fakeReviewer) SubmitNewReview(_ domain.PullRequestHandle, _ domain.ReviewEvent, _ string) error {
	f.calls = append(f.calls, "SubmitNewReview")
	return f.submitErr
}

func (f *fakeReviewer) DiscardReview(_ domain.ReviewHandle) error {
	return nil
}

// GitHub allows one unsubmitted review per user per pull request.
func TestPostLineCommentStartsAReviewOnlyWhenThereIsNone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pendingID string
		newID     string
		wantCalls []string
		wantID    string
	}{
		{
			name:      "no pending review: start one first",
			pendingID: "",
			newID:     "REV_new",
			wantCalls: []string{"StartReview", "AddReviewThread(REV_new)"},
			wantID:    "REV_new",
		},
		{
			name:      "pending review already open: add straight to it",
			pendingID: "REV_open",
			wantCalls: []string{"AddReviewThread(REV_open)"},
			wantID:    "REV_open",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeReviewer{newID: tc.newID}
			u := &Usecase{reviews: f}

			id, err := u.PostLineComment(
				domain.ReviewTarget{PullRequest: "PR_1", Pending: domain.ReviewHandle(tc.pendingID)},
				domain.PendingComment{Path: "a.go", Line: 1, Body: "nit"},
			)
			if err != nil {
				t.Fatalf("%s: PostLineComment: %v", tc.name, err)
			}
			if !slices.Equal(f.calls, tc.wantCalls) {
				t.Errorf("%s: calls = %v, want %v", tc.name, f.calls, tc.wantCalls)
			}
			// The caller reuses the id for the next comment, so a wrong one
			// means every later comment starts another review.
			if string(id) != tc.wantID {
				t.Errorf("%s: review = %q, want %q", tc.name, id, tc.wantID)
			}
		})
	}
}

// A failed StartReview must not be followed by an AddReviewThread against an
// empty id: that call succeeds against nothing and the comment disappears.
func TestPostLineCommentStopsWhenTheReviewCannotBeStarted(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	f := &fakeReviewer{startErr: boom}
	u := &Usecase{reviews: f}

	if _, err := u.PostLineComment(domain.ReviewTarget{PullRequest: "PR_1"}, domain.PendingComment{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
	if !slices.Equal(f.calls, []string{"StartReview"}) {
		t.Errorf("calls = %v, want the walk to stop at StartReview", f.calls)
	}
}

// Going through StartReview first would leave an empty pending review
// behind if the submission then failed.
func TestSubmitReviewPicksTheOneCallThatFitsTheTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		pendingID string
		wantCalls []string
	}{
		{"comments waiting: submit the pending review", "REV_open", []string{"SubmitReview"}},
		{"nothing waiting: create and submit in one call", "", []string{"SubmitNewReview"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			f := &fakeReviewer{}
			u := &Usecase{reviews: f}

			err := u.SubmitReview(
				domain.ReviewTarget{PullRequest: "PR_1", Pending: domain.ReviewHandle(tc.pendingID)},
				domain.EventApprove, "lgtm",
			)
			if err != nil {
				t.Fatalf("%s: SubmitReview: %v", tc.name, err)
			}
			if !slices.Equal(f.calls, tc.wantCalls) {
				t.Errorf("%s: calls = %v, want %v", tc.name, f.calls, tc.wantCalls)
			}
		})
	}
}
