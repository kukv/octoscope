package usecase

import (
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

type fakeReviewer struct {
	newID     string
	threadErr error
	submitErr error
	gotTarget domain.ReviewTarget
}

func (f *fakeReviewer) AddReviewThread(t domain.ReviewTarget, _ domain.PendingComment) (domain.ReviewHandle, error) {
	f.gotTarget = t
	if f.threadErr != nil {
		return "", f.threadErr
	}
	return domain.ReviewHandle(f.newID), nil
}

func (f *fakeReviewer) SubmitReview(t domain.ReviewTarget, _ domain.ReviewEvent, _ string) error {
	f.gotTarget = t
	return f.submitErr
}

func (f *fakeReviewer) DiscardReview(_ domain.ReviewHandle) error {
	return nil
}

// PostLineComment answers whichever review the gateway put the comment on:
// which requests that takes is the gateway's decision, not the usecase's.
func TestPostLineCommentAnswersTheGatewaysHandle(t *testing.T) {
	t.Parallel()

	f := &fakeReviewer{newID: "REV_new"}
	u := &Usecase{reviews: f}

	tgt := domain.ReviewTarget{PullRequest: "PR_1"}
	id, err := u.PostLineComment(tgt, domain.PendingComment{Path: "a.go", Line: 1, Body: "nit"})
	if err != nil {
		t.Fatalf("PostLineComment: %v", err)
	}
	if id != "REV_new" {
		t.Errorf("id = %q, want REV_new", id)
	}
	if f.gotTarget != tgt {
		t.Errorf("target passed to gateway = %+v, want %+v", f.gotTarget, tgt)
	}
}

func TestPostLineCommentWrapsTheGatewaysError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	f := &fakeReviewer{threadErr: boom}
	u := &Usecase{reviews: f}

	if _, err := u.PostLineComment(domain.ReviewTarget{PullRequest: "PR_1"}, domain.PendingComment{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}

func TestSubmitReviewPassesTheTargetThrough(t *testing.T) {
	t.Parallel()

	f := &fakeReviewer{}
	u := &Usecase{reviews: f}

	tgt := domain.ReviewTarget{PullRequest: "PR_1", Pending: "REV_open"}
	if err := u.SubmitReview(tgt, domain.EventApprove, "lgtm"); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if f.gotTarget != tgt {
		t.Errorf("target passed to gateway = %+v, want %+v", f.gotTarget, tgt)
	}
}

func TestSubmitReviewWrapsTheGatewaysError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	f := &fakeReviewer{submitErr: boom}
	u := &Usecase{reviews: f}

	if err := u.SubmitReview(domain.ReviewTarget{PullRequest: "PR_1"}, domain.EventApprove, "lgtm"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}
