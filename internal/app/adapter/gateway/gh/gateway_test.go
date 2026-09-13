package gh

import (
	"github.com/kukv/octoscope/internal/github/api"
	"github.com/kukv/octoscope/internal/github/cli"
)

// Both clients have to answer everything the gateway promotes or overrides.
// A conversion that changes backend's signature without changing the client
// breaks here, at compile time, rather than at the call site.
var (
	_ backend = (*cli.Client)(nil)
	_ backend = (*api.Client)(nil)
)
