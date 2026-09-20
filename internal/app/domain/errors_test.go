package domain_test

import (
	"errors"
	"fmt"
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
