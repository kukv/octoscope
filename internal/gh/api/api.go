// Package api fetches GitHub data by talking to the API itself, for machines
// that have no gh CLI. It answers the same domain types internal/gh/cli does
// and sends the same GraphQL documents, through a different transport.
package api

import (
	"os"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

// Token reads the token this backend authenticates with. The order is gh's
// own: GH_TOKEN wins so that a machine with both can point octoscope at the
// same credential gh uses.
func Token() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, nil
		}
	}
	return "", gh.ErrUnauthenticated
}
