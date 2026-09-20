package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// fakeItems records what each item operation was handed. Which of GitHub's
// two APIs answers is the gateway's business, so there is nothing here to
// tell a pull request from an issue: these tests only watch the arguments
// survive the trip.
type fakeItems struct {
	item domain.Item
	err  error

	gotRef     domain.ItemRef
	gotBody    string
	gotClosing bool
	gotAdd     []string
	gotRemove  []string
}

func (f *fakeItems) GetItem(_ context.Context, ref domain.ItemRef) (domain.Item, error) {
	f.gotRef = ref
	return f.item, f.err
}

func (f *fakeItems) AddComment(_ context.Context, ref domain.ItemRef, body string) error {
	f.gotRef, f.gotBody = ref, body
	return f.err
}

func (f *fakeItems) SetState(_ context.Context, ref domain.ItemRef, closing bool) error {
	f.gotRef, f.gotClosing = ref, closing
	return f.err
}

func (f *fakeItems) EditLabels(_ context.Context, ref domain.ItemRef, add, remove []string) error {
	f.gotRef, f.gotAdd, f.gotRemove = ref, add, remove
	return f.err
}

func (f *fakeItems) EditAssignees(_ context.Context, ref domain.ItemRef, add, remove []string) error {
	f.gotRef, f.gotAdd, f.gotRemove = ref, add, remove
	return f.err
}

func itemRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12}
}

func TestGetItemAnswersWhatTheLayerReturned(t *testing.T) {
	t.Parallel()

	want := domain.Item{Ref: itemRef(), Title: "a pr"}
	f := &fakeItems{item: want}

	got, err := (&Usecase{items: f}).GetItem(t.Context(), itemRef())
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if got.Ref != want.Ref || got.Title != want.Title {
		t.Errorf("item = %+v, want %+v", got, want)
	}
	if f.gotRef != itemRef() {
		t.Errorf("the reference reached the layer as %+v", f.gotRef)
	}
}

// TestGetItemPassesTheFetchFailureThrough is why the failure is compared with
// == rather than errors.Is: this layer no longer names the kind, so it has no
// context of its own to add, and adding one back would need the kind branch
// the gateway now owns.
func TestGetItemPassesTheFetchFailureThrough(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")

	if _, err := (&Usecase{items: &fakeItems{err: want}}).GetItem(t.Context(), itemRef()); err != want {
		t.Errorf("err = %v, want exactly %v", err, want)
	}
}

func TestAddCommentPassesTheBodyThrough(t *testing.T) {
	t.Parallel()

	f := &fakeItems{}
	if err := (&Usecase{comments: f}).AddComment(t.Context(), itemRef(), "looks good"); err != nil {
		t.Fatalf("AddComment: %v", err)
	}
	if f.gotRef != itemRef() || f.gotBody != "looks good" {
		t.Errorf("the layer got (%+v, %q)", f.gotRef, f.gotBody)
	}
}

func TestSetStatePassesTheDirectionThrough(t *testing.T) {
	t.Parallel()

	for _, closing := range []bool{true, false} {
		f := &fakeItems{}
		if err := (&Usecase{states: f}).SetState(t.Context(), itemRef(), closing); err != nil {
			t.Fatalf("SetState(%v): %v", closing, err)
		}
		if f.gotRef != itemRef() || f.gotClosing != closing {
			t.Errorf("the layer got (%+v, closing=%v), want closing=%v", f.gotRef, f.gotClosing, closing)
		}
	}
}

func TestEditLabelsPassesBothListsThrough(t *testing.T) {
	t.Parallel()

	f := &fakeItems{}
	if err := (&Usecase{labels: f}).EditLabels(t.Context(), itemRef(), []string{"bug"}, []string{"wip"}); err != nil {
		t.Fatalf("EditLabels: %v", err)
	}
	if f.gotRef != itemRef() || !slices.Equal(f.gotAdd, []string{"bug"}) || !slices.Equal(f.gotRemove, []string{"wip"}) {
		t.Errorf("the layer got (%+v, add=%v, remove=%v)", f.gotRef, f.gotAdd, f.gotRemove)
	}
}

func TestEditAssigneesPassesBothListsThrough(t *testing.T) {
	t.Parallel()

	f := &fakeItems{}
	if err := (&Usecase{assignees: f}).EditAssignees(t.Context(), itemRef(), []string{"alice"}, []string{"bob"}); err != nil {
		t.Fatalf("EditAssignees: %v", err)
	}
	if f.gotRef != itemRef() || !slices.Equal(f.gotAdd, []string{"alice"}) || !slices.Equal(f.gotRemove, []string{"bob"}) {
		t.Errorf("the layer got (%+v, add=%v, remove=%v)", f.gotRef, f.gotAdd, f.gotRemove)
	}
}
