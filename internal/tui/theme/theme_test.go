package theme_test

import (
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// dark restores the default background assumption, because it is process-wide
// state (.claude/rules/testing.md).
func dark(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { theme.SetDark(true) })
}

func TestTheBackgroundChangesTheColour(t *testing.T) {
	dark(t)

	theme.SetDark(true)
	onDark := theme.Dim().Render("x")
	theme.SetDark(false)
	onLight := theme.Dim().Render("x")

	if onDark == onLight {
		t.Errorf("the same colour is used on both backgrounds: %q", onDark)
	}
	for _, s := range []string{onDark, onLight} {
		if !strings.Contains(s, "\x1b[") {
			t.Errorf("no colour was emitted at all: %q", s)
		}
	}
}

// TestEachReviewStateHasItsOwnColour is what makes the state colours worth
// having: two states that render the same are indistinguishable on screen.
func TestEachReviewStateHasItsOwnColour(t *testing.T) {
	dark(t)

	seen := map[string]string{}
	for name, s := range map[string]gh.ReviewState{
		"approved":          gh.ReviewApproved,
		"changes requested": gh.ReviewChangesRequested,
		"review required":   gh.ReviewRequired,
	} {
		got := theme.Review(s, false).Render("x")
		if other, clash := seen[got]; clash {
			t.Errorf("%s and %s render identically: %q", name, other, got)
		}
		seen[got] = name
	}

	if theme.Review(gh.ReviewApproved, true).Render("x") == theme.Review(gh.ReviewApproved, false).Render("x") {
		t.Error("a draft is coloured as though it were waiting on a review")
	}
}

func TestEachCheckStateHasItsOwnColour(t *testing.T) {
	dark(t)

	seen := map[string]string{}
	for name, s := range map[string]gh.CheckState{
		"success": gh.CheckSuccess,
		"failure": gh.CheckFailure,
		"running": gh.CheckRunning,
		"none":    gh.CheckNone,
	} {
		got := theme.Check(s).Render("x")
		if other, clash := seen[got]; clash {
			t.Errorf("%s and %s render identically: %q", name, other, got)
		}
		seen[got] = name
	}
}

func TestBadgeChoosesReadableTextForTheLabelColour(t *testing.T) {
	dark(t)

	tests := []struct {
		name, hex, wantText string
	}{
		{"a pale label takes black text", "d4c5f9", "0;0;0"},
		{"a dark label takes white text", "0e8a16", "255;255;255"},
		{"white takes black text", "ffffff", "0;0;0"},
		{"black takes white text", "000000", "255;255;255"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := theme.Badge(tc.hex).Render("bug")
			if !strings.Contains(got, tc.wantText) {
				t.Errorf("Badge(%q) = %q, want foreground %s", tc.hex, got, tc.wantText)
			}
			if !strings.Contains(got, "48;2;") {
				t.Errorf("Badge(%q) = %q, want a filled background", tc.hex, got)
			}
		})
	}
}

func TestAnUnusableLabelColourStillRendersTheName(t *testing.T) {
	dark(t)

	for _, hex := range []string{"", "xyz", "1234567", "gggggg"} {
		got := theme.Badge(hex).Render("bug")
		if !strings.Contains(got, "bug") {
			t.Errorf("Badge(%q) lost the label name: %q", hex, got)
		}
		if strings.Contains(got, "48;2;") {
			t.Errorf("Badge(%q) filled a background from a colour it could not read: %q", hex, got)
		}
	}
}
