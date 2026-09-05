package icon_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/tui/icon"
)

func TestReview(t *testing.T) {
	tests := []struct {
		name  string
		state gh.ReviewState
		draft bool
		want  string
	}{
		{"draft wins over approved", gh.ReviewApproved, true, "◌"},
		{"draft wins over changes requested", gh.ReviewChangesRequested, true, "◌"},
		{"none", gh.ReviewNone, false, "•"},
		{"required", gh.ReviewRequired, false, "•"},
		{"approved", gh.ReviewApproved, false, "✓"},
		{"changes requested", gh.ReviewChangesRequested, false, "×"},
	}
	for _, tt := range tests {
		if got := icon.Review(tt.state, tt.draft); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestCheck(t *testing.T) {
	tests := []struct {
		name  string
		state gh.CheckState
		want  string
	}{
		{"none", gh.CheckNone, " "},
		{"pending", gh.CheckPending, "◍"},
		{"running", gh.CheckRunning, "◍"},
		{"success", gh.CheckSuccess, "✓"},
		{"failure", gh.CheckFailure, "×"},
	}
	for _, tt := range tests {
		if got := icon.Check(tt.state); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestChecksBar(t *testing.T) {
	tests := []struct {
		name      string
		checks    gh.Checks
		wantEmpty bool
		wantFill  int // -1 means "don't check the exact count"
	}{
		{"no checks is empty", gh.Checks{Total: 0}, true, -1},
		{"none passed has no filled cells", gh.Checks{Total: 10, Passed: 0}, false, 0},
		{"tiny ratio still shows one filled cell", gh.Checks{Total: 100, Passed: 1}, false, 1},
		{"all passed", gh.Checks{Total: 7, Passed: 7}, false, 7},
		{"partial", gh.Checks{Total: 4, Passed: 2}, false, -1},
	}
	for _, tt := range tests {
		done, rest := icon.ChecksBar(tt.checks)
		if tt.wantEmpty {
			if done != "" || rest != "" {
				t.Errorf("%s: got %q and %q, want both empty", tt.name, done, rest)
			}
			continue
		}
		if w := ansi.StringWidth(done + rest); w != icon.BarWidth {
			t.Errorf("%s: width = %d, want %d", tt.name, w, icon.BarWidth)
		}
		if strings.ContainsAny(rest, "▰") {
			t.Errorf("%s: the unfinished half carries a filled cell: %q", tt.name, rest)
		}
		filled := strings.Count(done, "▰")
		if tt.wantFill >= 0 && filled != tt.wantFill {
			t.Errorf("%s: filled = %d, want %d", tt.name, filled, tt.wantFill)
		}
		if tt.checks.Passed == 0 && filled != 0 {
			t.Errorf("%s: expected no filled cells when Passed == 0, got %d", tt.name, filled)
		}
	}
}
