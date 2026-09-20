package usecase

import (
	"context"
	"errors"
	"testing"
)

// fakeViewer is the viewer port: one login, or the failure to read it.
type fakeViewer struct {
	viewer    string
	viewerErr error
}

func (f *fakeViewer) Viewer(context.Context) (string, error) { return f.viewer, f.viewerErr }

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
