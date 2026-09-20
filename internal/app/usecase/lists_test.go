package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// fakeViewer is the viewer port: one login, or the failure to read it.
type fakeViewer struct {
	viewer    string
	viewerErr error
}

func (f *fakeViewer) Viewer(context.Context) (string, error) { return f.viewer, f.viewerErr }

// fakeItemLister records the query ListItems was asked to run.
type fakeItemLister struct {
	items []domain.Item
	err   error

	gotRepo string
	gotKind domain.ItemKind
}

func (f *fakeItemLister) ListItems(_ context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error) {
	f.gotRepo, f.gotKind = repo, kind
	return f.items, f.err
}

// TestListItemsPassesTheKindThrough is what makes the kind a parameter of the
// query rather than a branch: this layer hands it on without reading it.
func TestListItemsPassesTheKindThrough(t *testing.T) {
	t.Parallel()

	f := &fakeItemLister{items: []domain.Item{{Title: "an issue"}}}

	got, err := (&Usecase{itemLists: f}).ListItems(t.Context(), "kukv/octoscope", domain.ItemIssue)
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if len(got) != 1 || got[0].Title != "an issue" {
		t.Errorf("items = %+v, want the one the layer returned", got)
	}
	if f.gotRepo != "kukv/octoscope" || f.gotKind != domain.ItemIssue {
		t.Errorf("the layer got (%q, %v)", f.gotRepo, f.gotKind)
	}
}

func TestListItemsPassesTheFailureThrough(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")

	if _, err := (&Usecase{itemLists: &fakeItemLister{err: want}}).ListItems(t.Context(), "", domain.ItemPR); err != want {
		t.Errorf("err = %v, want exactly %v", err, want)
	}
}

// TestViewerPassesTheLoginThrough is the whole of what this layer does with
// it: there is nothing to decide, and a view is not allowed to reach the
// GitHub layer itself (.claude/rules/architecture.md).
func TestViewerPassesTheLoginThrough(t *testing.T) {
	t.Parallel()

	u := &Usecase{viewer: &fakeViewer{viewer: "kukv"}}

	got, err := u.Viewer(t.Context())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if got != "kukv" {
		t.Errorf("login is %q, want %q", got, "kukv")
	}
}

func TestViewerPassesTheFailureThrough(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	u := &Usecase{viewer: &fakeViewer{viewerErr: want}}

	if _, err := u.Viewer(t.Context()); !errors.Is(err, want) {
		t.Errorf("Viewer: %v, want %v", err, want)
	}
}
