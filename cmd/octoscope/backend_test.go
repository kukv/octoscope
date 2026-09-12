package main

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// gh wins when it is there: it carries the user's own login, which the
// environment variables may not have, and it is what the user already set up.
func TestGhOnThePathWins(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope",
		func(string) (string, error) { return "/usr/bin/gh", nil },
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if c == nil || a != nil {
		t.Errorf("chose the API backend with gh on the path")
	}
}

// Without gh the token is the whole reason this backend exists.
func TestATokenIsUsedWhenGhIsNotThere(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope",
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if a == nil || c != nil {
		t.Errorf("did not choose the API backend without gh")
	}
}

// Neither one is not a failure to report as an unknown error: it is the one
// situation the user can fix, and the caller tells them how.
func TestNeitherGhNorATokenIsAnAuthenticationFailure(t *testing.T) {
	t.Parallel()

	_, _, err := chooseBackend("/work", "kukv/octoscope",
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "", gh.ErrUnauthenticated })
	if !errors.Is(err, gh.ErrUnauthenticated) {
		t.Errorf("err = %v, want gh.ErrUnauthenticated", err)
	}
}
