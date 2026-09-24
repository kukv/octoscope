package main

import (
	"fmt"
	"strings"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/api"
	"github.com/kukv/octoscope/internal/github/cli"
)

// backendMode is which client the user asked to talk to GitHub through.
type backendMode string

const (
	// backendAuto takes gh when it is on PATH and a token when it is not.
	backendAuto backendMode = "auto"
	// backendGh takes gh and nothing else.
	backendGh backendMode = "gh"
	// backendAPI takes a token and never looks for gh.
	backendAPI backendMode = "api"
)

// backendEnvVar names the environment variable that chooses a backend,
// between --backend and the settings file.
const backendEnvVar = "OCTOSCOPE_BACKEND"

// unknownBackendError is a backend value that is none of the three. Source
// is where it was written, which is where the user has to fix it.
type unknownBackendError struct {
	Source string
	Value  string
}

func (e *unknownBackendError) Error() string {
	return fmt.Sprintf("unknown backend %q (%s)", e.Value, e.Source)
}

// resolveBackend picks the backend from --backend, the environment and the
// settings file, in that order; an empty one defers to the next.
//
// An unknown value is an error where it stands rather than a fall back to the
// next place or to auto, unlike --icons and default_tab: nothing on screen
// tells the two backends apart, so a typo would otherwise go unnoticed.
func resolveBackend(flag, env, configured string) (backendMode, error) {
	for _, c := range []struct{ source, value string }{
		{"--backend", flag},
		{backendEnvVar, env},
		{"config.yaml", configured},
	} {
		switch m := backendMode(strings.ToLower(strings.TrimSpace(c.value))); m {
		case "":
			continue
		case backendAuto, backendGh, backendAPI:
			return m, nil
		}
		return "", &unknownBackendError{Source: c.source, Value: c.value}
	}
	return backendAuto, nil
}

// chooseBackend picks which client talks to GitHub. Under auto gh comes
// first: it is already signed in, and it carries a login the environment
// variables need not have. Without it a token is the whole reason the API
// backend exists. gh and api take only the one they name, so a machine with
// gh on PATH can still be pointed at the API backend.
//
// lookPath and token are parameters rather than the functions themselves so a
// test can say what the machine has.
//
// Exactly one of the two clients is non-nil when err is nil. Two return values
// rather than one interface is deliberate: the caller passes whichever it got
// to usecase.New, and the compiler checks both clients answer everything the
// usecase layer asks for.
func chooseBackend(dir, repo string, mode backendMode,
	lookPath func(string) (string, error), token func() (string, error),
) (*cli.Client, *api.Client, error) {
	if mode != backendAPI {
		_, err := lookPath("gh")
		if err == nil {
			return cli.New(dir, repo), nil, nil
		}
		if mode == backendGh {
			return nil, nil, fmt.Errorf("%w: %w", domain.ErrBackendUnavailable, err)
		}
	}
	t, err := token()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	return nil, api.New(dir, repo, t), nil
}

// backendFailure is the message ID for chooseBackend failing under mode.
// Under auto either remedy works; under gh or api only the one it names does.
func backendFailure(mode backendMode) string {
	switch mode {
	case backendGh:
		return "error.gh_not_found"
	case backendAPI:
		return "error.no_token"
	default:
		return "error.no_backend"
	}
}
