package usecase

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

type fakeReviewer struct {
	newID     string
	newThread domain.ReviewThread
	threadErr error
	submitErr error
	gotTarget domain.ReviewTarget
}

func (f *fakeReviewer) AddReviewThread(_ context.Context, t domain.ReviewTarget, _ domain.PendingComment) (domain.ReviewHandle, domain.ReviewThread, error) {
	f.gotTarget = t
	if f.threadErr != nil {
		return "", domain.ReviewThread{}, f.threadErr
	}
	return domain.ReviewHandle(f.newID), f.newThread, nil
}

func (f *fakeReviewer) SubmitReview(_ context.Context, t domain.ReviewTarget, _ domain.ReviewEvent, _ string) error {
	f.gotTarget = t
	return f.submitErr
}

func (f *fakeReviewer) DiscardReview(_ context.Context, _ domain.ReviewHandle) error {
	return nil
}

// PostLineComment answers whichever review the gateway put the comment on,
// and the thread it created: which requests that takes is the gateway's
// decision, not the usecase's, and the diff view draws that thread rather
// than refetching every one.
func TestPostLineCommentAnswersTheGatewaysHandle(t *testing.T) {
	t.Parallel()

	thread := domain.ReviewThread{Path: "a.go", Line: 1, Comments: []domain.ThreadComment{{Body: "nit", Pending: true}}}
	f := &fakeReviewer{newID: "REV_new", newThread: thread}
	u := &Usecase{reviews: f}

	tgt := domain.ReviewTarget{PullRequest: "PR_1"}
	id, got, err := u.PostLineComment(t.Context(), tgt, domain.PendingComment{Path: "a.go", Line: 1, Body: "nit"})
	if err != nil {
		t.Fatalf("PostLineComment: %v", err)
	}
	if id != "REV_new" {
		t.Errorf("id = %q, want REV_new", id)
	}
	if !reflect.DeepEqual(got, thread) {
		t.Errorf("thread = %+v, want %+v", got, thread)
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

	if _, _, err := u.PostLineComment(t.Context(), domain.ReviewTarget{PullRequest: "PR_1"}, domain.PendingComment{}); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}

func TestSubmitReviewPassesTheTargetThrough(t *testing.T) {
	t.Parallel()

	f := &fakeReviewer{}
	u := &Usecase{reviews: f}

	tgt := domain.ReviewTarget{PullRequest: "PR_1", Pending: "REV_open"}
	if err := u.SubmitReview(t.Context(), tgt, domain.EventApprove, "lgtm"); err != nil {
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

	if err := u.SubmitReview(t.Context(), domain.ReviewTarget{PullRequest: "PR_1"}, domain.EventApprove, "lgtm"); !errors.Is(err, boom) {
		t.Errorf("err = %v, want it to wrap %v", err, boom)
	}
}
