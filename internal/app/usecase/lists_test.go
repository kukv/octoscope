package usecase

import (
	"errors"
	"testing"
)

// TestViewerPassesTheLoginThrough is the whole of what this layer does with
// it: there is nothing to decide, and a view is not allowed to reach the
// GitHub layer itself (.claude/rules/architecture.md).
func TestViewerPassesTheLoginThrough(t *testing.T) {
	t.Parallel()

	u := &Usecase{viewer: &fakeSource{viewer: "kukv"}}

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
	u := &Usecase{viewer: &fakeSource{viewerErr: want}}

	if _, err := u.Viewer(t.Context()); !errors.Is(err, want) {
		t.Errorf("Viewer: %v, want %v", err, want)
	}
}
