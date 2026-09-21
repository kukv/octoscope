package domain_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// What the user must act on takes the whole screen; everything else costs
// them a line. Getting this wrong either hides a failure they can fix or
// throws a board away over a 502.
func TestIsFatalOnlyForWhatTheUserMustActOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"backend unavailable", domain.ErrBackendUnavailable, true},
		{"not signed in", fmt.Errorf("gh pr list: %w", domain.ErrUnauthenticated), true},
		{"the backend did not answer", fmt.Errorf("gh pr list: %w", domain.ErrTransient), false},
		{"anything else", errors.New("gh: HTTP 404"), false},
	}
	for _, tt := range tests {
		if got := domain.IsFatal(tt.err); got != tt.want {
			t.Errorf("%s: IsFatal = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// A domain sentinel names what kind of failure it is and stops there. Two
// things it must not name.
//
// The remedy ("install gh and run gh auth login") belongs to the UI, which
// has the user's language: i18n's error.gh_not_found and
// error.unauthenticated are what the error screen actually shows (root.go
// showError). A sentinel that carries the remedy is a second, untranslated
// copy that nothing displays.
//
// The service belongs to the gateway. "GitHub did not answer" says which
// backend was talking, and the domain is the layer that does not know --
// the same sentence is wrong the moment the answer came from somewhere
// else. internal/github's own sentinels may say it; these may not.
func TestSentinelsNameTheKindAndNotTheRemedy(t *testing.T) {
	t.Parallel()

	banned := []string{
		// remedies
		"gh CLI", "gh auth", "install", "run:",
		// services
		"GitHub", "gh ", "GitLab",
	}
	for _, err := range []error{
		domain.ErrBackendUnavailable,
		domain.ErrTransient,
		domain.ErrUnauthenticated,
	} {
		for _, b := range banned {
			if strings.Contains(err.Error(), b) {
				t.Errorf("%q names %q; the remedy belongs in i18n and the service in the gateway", err.Error(), b)
			}
		}
	}
}
