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
		{"GitHub did not answer", fmt.Errorf("gh pr list: %w", domain.ErrTransient), false},
		{"anything else", errors.New("gh: HTTP 404"), false},
	}
	for _, tt := range tests {
		if got := domain.IsFatal(tt.err); got != tt.want {
			t.Errorf("%s: IsFatal = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// A domain sentinel names what kind of failure it is and stops there. The
// remedy ("install gh and run gh auth login") belongs to the UI, which has
// the user's language: i18n's error.gh_not_found and error.unauthenticated
// are what the error screen actually shows (root.go showError). A sentinel
// that carries the remedy is a second, untranslated copy that nothing
// displays -- and that goes stale the moment a second backend exists.
func TestSentinelsNameTheKindAndNotTheRemedy(t *testing.T) {
	t.Parallel()

	remedies := []string{"gh CLI", "gh auth", "install", "run:"}
	for _, err := range []error{
		domain.ErrBackendUnavailable,
		domain.ErrUnauthenticated,
	} {
		for _, r := range remedies {
			if strings.Contains(err.Error(), r) {
				t.Errorf("%q carries a remedy (%q); it belongs in i18n", err.Error(), r)
			}
		}
	}
}
