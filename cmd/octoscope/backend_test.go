package main

import (
	"errors"
	"os/exec"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// gh wins when it is there: it carries the user's own login, which the
// environment variables may not have, and it is what the user already set up.
func TestGhOnThePathWins(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope", false,
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

	c, a, err := chooseBackend("/work", "kukv/octoscope", false,
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

	_, _, err := chooseBackend("/work", "kukv/octoscope", false,
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "", domain.ErrUnauthenticated })
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("err = %v, want domain.ErrUnauthenticated", err)
	}
}

// token's own error is the reason chooseBackend concluded there is no
// authentication at all, and it is lost the moment token grows a failure mode
// other than domain.ErrUnauthenticated itself. Discarding it here would leave that
// future failure with nothing to tell errors.Is or the user apart from the
// sentinel's own text.
func TestTheUnderlyingTokenErrorSurvives(t *testing.T) {
	t.Parallel()

	cause := errors.New("keyring is locked")
	_, _, err := chooseBackend("/work", "kukv/octoscope", false,
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "", cause })
	if !errors.Is(err, cause) {
		t.Errorf("err = %v, want it to wrap %v", err, cause)
	}
}

// --api, then OCTOSCOPE_API, then the settings file. A false written in a
// higher place is an answer, not an absence: it is how a user who set api in
// the settings file gets gh back for one run.
func TestResolveAPIOrder(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		flagSet    bool
		flagValue  bool
		env        string
		configured bool
		want       bool
	}{
		{"nothing set keeps today's choice", false, false, "", false, false},
		{"the settings file on its own", false, false, "", true, true},
		{"the environment on its own", false, false, "1", false, true},
		{"the flag on its own", true, true, "", false, true},
		{"--api=false beats the environment and the settings file", true, false, "true", true, false},
		{"OCTOSCOPE_API=0 beats the settings file", false, false, "0", true, false},
		{"the flag beats OCTOSCOPE_API=0", true, true, "0", false, true},
		{"a blank environment variable reads as unset", false, false, "  ", true, true},
		{"case and space are not a typo", false, false, " TRUE ", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveAPI(tc.flagSet, tc.flagValue, tc.env, tc.configured)
			if err != nil {
				t.Fatalf("resolveAPI: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveAPI(%v, %v, %q, %v) = %v, want %v",
					tc.flagSet, tc.flagValue, tc.env, tc.configured, got, tc.want)
			}
		})
	}
}

// Nothing on screen says which backend is running, so a typo that quietly fell
// through to the settings file or to gh would never be noticed. It stops the
// start instead.
func TestAnUnreadableOCTOSCOPEAPIIsAnError(t *testing.T) {
	t.Parallel()

	if _, err := resolveAPI(false, false, "yes", true); err == nil {
		t.Error("resolveAPI accepted OCTOSCOPE_API=yes")
	}
}

// --api exists for a machine where gh is installed but should not be used, so
// gh is not even looked for.
func TestUseAPISkipsGhEvenWhenItIsThere(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope", true,
		func(string) (string, error) {
			t.Error("looked for gh although the API backend was asked for")
			return "/usr/bin/gh", nil
		},
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if a == nil || c != nil {
		t.Errorf("did not choose the API backend when asked for it")
	}
}

// Asking for the API backend without a token has no gh to fall back to: it is
// the same authentication failure as having neither.
func TestUseAPIWithoutATokenIsAnAuthenticationFailure(t *testing.T) {
	t.Parallel()

	_, _, err := chooseBackend("/work", "kukv/octoscope", true,
		func(string) (string, error) { return "/usr/bin/gh", nil },
		func() (string, error) { return "", domain.ErrUnauthenticated })
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("err = %v, want domain.ErrUnauthenticated", err)
	}
}
