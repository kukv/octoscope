package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/api"
	"github.com/kukv/octoscope/internal/github/cli"
)

// apiEnvVar names the environment variable that takes the API backend,
// between --api and the settings file.
const apiEnvVar = "OCTOSCOPE_API"

// resolveAPI decides whether to take the API backend: --api when it was given
// at all, then OCTOSCOPE_API when it is set, then the settings file.
//
// flagSet says whether --api was on the command line, so that --api=false can
// overrule the two below it. env is OCTOSCOPE_API's value, read by the caller
// so a test need not touch the environment.
//
// Only env can be unreadable here: the flag package and the YAML decoder
// check the other two. Unlike --icons, an unreadable value is an error rather
// than a fall back, because nothing on screen tells the two backends apart.
func resolveAPI(flagSet, flagValue bool, env string, configured bool) (bool, error) {
	if flagSet {
		return flagValue, nil
	}
	if v := strings.TrimSpace(env); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("%s: %w", apiEnvVar, err)
		}
		return b, nil
	}
	return configured, nil
}

// chooseBackend picks which client talks to GitHub. gh comes first: it is
// already signed in, and it carries a login the environment variables need not
// have. Without it a token is the whole reason the API backend exists.
//
// useAPI skips gh altogether, for a machine where gh is installed but should
// not be the way to GitHub. Only a token is tried then.
//
// lookPath and token are parameters rather than the functions themselves so a
// test can say what the machine has.
//
// Exactly one of the two clients is non-nil when err is nil. Two return values
// rather than one interface is deliberate: the caller passes whichever it got
// to usecase.New, and the compiler checks both clients answer everything the
// usecase layer asks for.
func chooseBackend(dir, repo string, useAPI bool,
	lookPath func(string) (string, error), token func() (string, error),
) (*cli.Client, *api.Client, error) {
	if !useAPI {
		if _, err := lookPath("gh"); err == nil {
			return cli.New(dir, repo), nil, nil
		}
	}
	t, err := token()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", domain.ErrUnauthenticated, err)
	}
	return nil, api.New(dir, repo, t), nil
}
