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

	c, a, err := chooseBackend("/work", "kukv/octoscope", backendAuto,
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

	c, a, err := chooseBackend("/work", "kukv/octoscope", backendAuto,
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

	_, _, err := chooseBackend("/work", "kukv/octoscope", backendAuto,
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
	_, _, err := chooseBackend("/work", "kukv/octoscope", backendAuto,
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "", cause })
	if !errors.Is(err, cause) {
		t.Errorf("err = %v, want it to wrap %v", err, cause)
	}
}

func TestResolveBackend(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name                  string
		flag, env, configured string
		want                  backendMode
	}{
		{"nothing set", "", "", "", backendAuto},
		{"the flag wins", "api", "gh", "gh", backendAPI},
		{"the environment beats the settings file", "", "gh", "api", backendGh},
		{"the settings file on its own", "", "", "api", backendAPI},
		{"auto said out loud", "auto", "api", "", backendAuto},
		{"case and space are not a typo", " GH ", "", "", backendGh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveBackend(tc.flag, tc.env, tc.configured)
			if err != nil {
				t.Fatalf("resolveBackend: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveBackend(%q, %q, %q) = %q, want %q",
					tc.flag, tc.env, tc.configured, got, tc.want)
			}
		})
	}
}

// An unknown value stops at the place it was written instead of falling
// through to the next one: nothing on screen says which backend is running,
// so a typo that quietly became auto would never be noticed. The error names
// where the value came from, since that is where the user has to fix it.
func TestAnUnknownBackendNamesWhereItCameFrom(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		flag, env, configured string
		source, value         string
	}{
		{"cli", "", "", "--backend", "cli"},
		{"", "cli", "api", "OCTOSCOPE_BACKEND", "cli"},
		{"", "", "cli", "config.yaml", "cli"},
	} {
		_, err := resolveBackend(tc.flag, tc.env, tc.configured)
		var unknown *unknownBackendError
		if !errors.As(err, &unknown) {
			t.Errorf("resolveBackend(%q, %q, %q) err = %v, want an unknownBackendError",
				tc.flag, tc.env, tc.configured, err)
			continue
		}
		if unknown.Source != tc.source || unknown.Value != tc.value {
			t.Errorf("resolveBackend(%q, %q, %q) = %+v, want source %q value %q",
				tc.flag, tc.env, tc.configured, unknown, tc.source, tc.value)
		}
	}
}

func TestBackendGhUsesGh(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope", backendGh,
		func(string) (string, error) { return "/usr/bin/gh", nil },
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if c == nil || a != nil {
		t.Errorf("did not choose gh when asked for it")
	}
}

// Asking for gh is asking not to be handed the token backend in its place.
func TestBackendGhIgnoresAToken(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope", backendGh,
		func(string) (string, error) { return "", exec.ErrNotFound },
		func() (string, error) { return "a-token", nil })
	if c != nil || a != nil {
		t.Errorf("returned a client without gh on the path")
	}
	if !errors.Is(err, domain.ErrBackendUnavailable) {
		t.Errorf("err = %v, want domain.ErrBackendUnavailable", err)
	}
}

func TestBackendAPISkipsGh(t *testing.T) {
	t.Parallel()

	c, a, err := chooseBackend("/work", "kukv/octoscope", backendAPI,
		func(string) (string, error) { return "/usr/bin/gh", nil },
		func() (string, error) { return "a-token", nil })
	if err != nil {
		t.Fatalf("chooseBackend: %v", err)
	}
	if a == nil || c != nil {
		t.Errorf("chose gh when asked for the API backend")
	}
}

func TestBackendAPIWithoutATokenIsUnauthenticated(t *testing.T) {
	t.Parallel()

	cause := errors.New("no token")
	_, _, err := chooseBackend("/work", "kukv/octoscope", backendAPI,
		func(string) (string, error) { return "/usr/bin/gh", nil },
		func() (string, error) { return "", cause })
	if !errors.Is(err, domain.ErrUnauthenticated) || !errors.Is(err, cause) {
		t.Errorf("err = %v, want domain.ErrUnauthenticated wrapping %v", err, cause)
	}
}

// What the user is told to do depends on what they asked for: under auto
// either remedy works, but under gh or api only one of them does.
func TestBackendFailure(t *testing.T) {
	t.Parallel()

	for mode, want := range map[backendMode]string{
		backendAuto: "error.no_backend",
		backendGh:   "error.gh_not_found",
		backendAPI:  "error.no_token",
	} {
		if got := backendFailure(mode); got != want {
			t.Errorf("backendFailure(%q) = %q, want %q", mode, got, want)
		}
	}
}
