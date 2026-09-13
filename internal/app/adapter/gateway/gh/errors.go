package gh

import (
	"errors"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
)

// wrap turns a client's sentinel into the domain's. The client names what
// went wrong in its own service's terms -- "the gh binary is not on PATH" --
// and the application only needs to know which of its own kinds that is.
//
// An error wrap does not recognise passes through unchanged: this is the one
// place every override's error travels through, so swallowing an unknown
// error here would hide it from every one of them.
func wrap(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, github.ErrNotInstalled):
		return domain.Classify(domain.ErrBackendUnavailable, err.Error())
	case errors.Is(err, github.ErrUnauthenticated):
		return domain.Classify(domain.ErrUnauthenticated, err.Error())
	case errors.Is(err, github.ErrTransient):
		return domain.Classify(domain.ErrTransient, err.Error())
	default:
		return err
	}
}
