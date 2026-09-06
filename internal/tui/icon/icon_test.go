package icon_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/tui/icon"
)

// allSets names every set, so the tests below cannot quietly stop covering
// one when a set is added.
var allSets = map[string]icon.Set{
	"unicode": icon.Unicode,
	"nerd":    icon.Nerd,
	"ascii":   icon.ASCII,
}

// use switches the set and puts it back afterwards: the choice is
// process-wide state (.claude/rules/testing.md).
func use(t *testing.T, s icon.Set) {
	t.Helper()
	icon.Use(s)
	t.Cleanup(func() { icon.Use(icon.Unicode) })
}

// markers is every glyph a set can draw, in one slice.
func markers() []string {
	var got []string
	for _, draft := range []bool{false, true} {
		for _, s := range []gh.ReviewState{
			gh.ReviewNone, gh.ReviewRequired, gh.ReviewApproved, gh.ReviewChangesRequested,
		} {
			got = append(got, icon.Review(s, draft))
		}
	}
	got = append(got, icon.Issue())
	for _, s := range []gh.CheckState{
		gh.CheckPending, gh.CheckRunning, gh.CheckSuccess, gh.CheckFailure,
	} {
		got = append(got, icon.Check(s))
	}
	done, rest := icon.ChecksBar(gh.Checks{Total: 2, Passed: 1})
	got = append(got, done, rest)
	got = append(got, icon.Collapsed(), icon.CommentBar(), icon.ThreadBadge())
	return got
}

// TestEveryGlyphIsOneColumn is what keeps a set usable: the board pads its
// columns by display width, and a glyph that measures two columns shifts
// every card beside it. Nerd Font glyphs live in the private use area, where
// width is not something to assume (spec 6.4).
func TestEveryGlyphIsOneColumn(t *testing.T) {
	for name, set := range allSets {
		t.Run(name, func(t *testing.T) {
			use(t, set)
			for _, g := range markers() {
				for _, r := range g {
					if w := ansi.StringWidth(string(r)); w != 1 {
						t.Errorf("%q is %d columns wide, want 1", string(r), w)
					}
				}
			}
		})
	}
}

// TestEachSetDrawsItsOwnGlyphs is the assertion that would fail if a set were
// declared but never reached: ASCII drawing "✓" means the fallback is not a
// fallback at all.
func TestEachSetDrawsItsOwnGlyphs(t *testing.T) {
	seen := map[string]string{}
	for name, set := range allSets {
		use(t, set)
		got := strings.Join(markers(), "")
		if other, clash := seen[got]; clash {
			t.Errorf("the %s set draws exactly what %s draws: %q", name, other, got)
		}
		seen[got] = name
	}

	use(t, icon.ASCII)
	for _, g := range markers() {
		for _, r := range g {
			if r > 127 {
				t.Errorf("the ASCII set draws %q, which is not ASCII", string(r))
			}
		}
	}
}

func TestResolve(t *testing.T) {
	tests := []struct {
		name, flag, env string
		want            icon.Set
	}{
		{"nothing given", "", "", icon.Unicode},
		{"the flag wins", "ascii", "nerd", icon.ASCII},
		{"the environment is used when the flag is absent", "", "nerd", icon.Nerd},
		{"auto means the set that needs no font", "auto", "", icon.Unicode},
		{"an auto flag still lets the environment decide", "auto", "ascii", icon.ASCII},
		{"case and spacing do not matter", " Nerd ", "", icon.Nerd},
		{"a typo falls back rather than breaking the board", "nerdfont", "", icon.Unicode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(icon.EnvVar, tt.env)
			if got := icon.Resolve(tt.flag); got != tt.want {
				t.Errorf("Resolve(%q) with %s=%q = %v, want %v",
					tt.flag, icon.EnvVar, tt.env, got, tt.want)
			}
		})
	}
}

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
