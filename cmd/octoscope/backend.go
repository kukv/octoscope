package main

import (
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/api"
	"github.com/kukv/octoscope/internal/gh/cli"
)

// chooseBackend picks which client talks to GitHub. gh comes first: it is
// already signed in, and it carries a login the environment variables need not
// have. Without it a token is the whole reason the API backend exists.
//
// lookPath and token are parameters rather than the functions themselves so a
// test can say what the machine has.
//
// Exactly one of the two clients is non-nil when err is nil. Two return values
// rather than one interface is deliberate: the caller passes whichever it got
// to usecase.New, and the compiler checks both clients answer everything the
// usecase layer asks for.
func chooseBackend(dir, repo string, lookPath func(string) (string, error),
	token func() (string, error),
) (*cli.Client, *api.Client, error) {
	if _, err := lookPath("gh"); err == nil {
		return cli.New(dir, repo), nil, nil
	}
	t, err := token()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", gh.ErrUnauthenticated, err)
	}
	return nil, api.New(dir, repo, t), nil
}
