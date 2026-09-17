package theme_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
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
	for name, s := range map[string]domain.ReviewState{
		"approved":          domain.ReviewApproved,
		"changes requested": domain.ReviewChangesRequested,
		"review required":   domain.ReviewRequired,
	} {
		got := theme.Review(s, false).Render("x")
		if other, clash := seen[got]; clash {
			t.Errorf("%s and %s render identically: %q", name, other, got)
		}
		seen[got] = name
	}

	if theme.Review(domain.ReviewApproved, true).Render("x") == theme.Review(domain.ReviewApproved, false).Render("x") {
		t.Error("a draft is coloured as though it were waiting on a review")
	}
}

// TestNothingReviewedYetStandsOut covers the two states that both draw the
// pending marker. GitHub reports no decision at all for a repository with no
// required review, and "review required" for one that has it; to whoever is
// looking at the board they are the same thing — nobody has looked yet — and
// the colour has to say so rather than leaving one of them as quiet as a
// draft.
func TestNothingReviewedYetStandsOut(t *testing.T) {
	dark(t)

	none := theme.Review(domain.ReviewNone, false).Render("x")
	if none == theme.Dim().Render("x") {
		t.Error("a pull request nobody has reviewed is as quiet as the text around it")
	}
	if want := theme.Review(domain.ReviewRequired, false).Render("x"); none != want {
		t.Errorf("the two unreviewed states are coloured apart: %q and %q", none, want)
	}
	// A draft is waiting on its author, not on a reviewer, and stays quiet.
	if theme.Review(domain.ReviewNone, true).Render("x") != theme.Dim().Render("x") {
		t.Error("a draft is coloured as though it were waiting on a reviewer")
	}
}

// TestAnIssueIsGreen covers the marker an issue carries. GitHub draws an open
// issue green, and the board has no other way to say that a card is an issue
// at all once its marker is as quiet as the text around it.
func TestAnIssueIsGreen(t *testing.T) {
	dark(t)

	issue := theme.Issue().Render("x")
	if issue == theme.Dim().Render("x") {
		t.Error("an issue is drawn as quietly as the text around it")
	}
	if want := theme.Review(domain.ReviewApproved, false).Render("x"); issue != want {
		t.Errorf("an issue is %q, want the same green as approved, %q", issue, want)
	}
}

func TestEachCheckStateHasItsOwnColour(t *testing.T) {
	dark(t)

	seen := map[string]string{}
	for name, s := range map[string]domain.CheckState{
		"success": domain.CheckSuccess,
		"failure": domain.CheckFailure,
		"running": domain.CheckRunning,
		"none":    domain.CheckNone,
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

func TestHighlightColoursCodeItKnows(t *testing.T) {
	got := theme.Highlight("walk.go", "func Walk() {}")
	if !strings.Contains(got, "\x1b[") {
		t.Errorf("Highlight returned no escapes for Go source: %q", got)
	}
	if ansi.Strip(got) != "func Walk() {}" {
		t.Errorf("Highlight changed the text: %q", ansi.Strip(got))
	}
}

func TestHighlightLeavesUnknownFilesAlone(t *testing.T) {
	const line = "some prose"
	if got := theme.Highlight("NOTES", line); got != line {
		t.Errorf("Highlight(%q) = %q, want it untouched", line, got)
	}
}

// TestHighlightFollowsTheBackground: the palette that reads on a dark
// terminal is unreadable on a light one, so the two must not come out the
// same.
func TestHighlightFollowsTheBackground(t *testing.T) {
	theme.SetDark(true)
	t.Cleanup(func() { theme.SetDark(true) })
	dark := theme.Highlight("walk.go", "func Walk() {}")
	theme.SetDark(false)
	light := theme.Highlight("walk.go", "func Walk() {}")
	if dark == light {
		t.Errorf("the same escapes on both backgrounds: %q", dark)
	}
}

// TestHighlightKeepsTheWidth is what stops highlighting from breaking every
// column downstream: ANSI escapes must not count towards the width, and the
// text must come back rune for rune, not smuggling newlines or other characters.
func TestHighlightKeepsTheWidth(t *testing.T) {
	cases := []struct {
		name string
		path string
		line string
	}{
		{name: "Go with tab", path: "walk.go", line: "func Walk() {}"},
		{name: "Go with Japanese", path: "walk.go", line: "\t// 日本語のコメント"},
		{name: "empty", path: "walk.go", line: ""},
		{name: "Makefile that triggers EnsureNL", path: "Makefile", line: "const x = 1"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := theme.Highlight(tc.path, tc.line)
			stripped := ansi.Strip(got)
			if stripped != tc.line {
				t.Errorf("Highlight(%q, %q) changed the text to %q", tc.path, tc.line, stripped)
			}
			if ansi.StringWidth(got) != ansi.StringWidth(tc.line) {
				t.Errorf("Highlight(%q, %q) is %d columns, want %d",
					tc.path, tc.line, ansi.StringWidth(got), ansi.StringWidth(tc.line))
			}
		})
	}
}

// openingBackground is the SGR sequence a line drawn by fillLine opens with.
// The test reads it out of the output rather than naming a colour: which
// colour a fill uses is theme's business and may change, but whatever it is
// must come back after every reset.
func openingBackground(t *testing.T, line string) string {
	t.Helper()
	if !strings.HasPrefix(line, "\x1b[") {
		t.Fatalf("the line does not open with a style: %q", line)
	}
	end := strings.IndexByte(line, 'm')
	if end < 0 {
		t.Fatalf("the opening style is unterminated: %q", line)
	}
	return line[:end+1]
}

// TestSelectedLineCarriesTheBackgroundPastALipglossReset is the whole point of
// the function: a reset clears the background along with the foreground, so a
// line with any colour in it would lose its fill from the first reset on.
func TestSelectedLineCarriesTheBackgroundPastALipglossReset(t *testing.T) {
	dark(t)

	line := theme.SelectedLine(theme.Dim().Render("x") + "y")
	bg := openingBackground(t, line)

	if n := strings.Count(line, ansi.ResetStyle+bg); n != 1 {
		t.Errorf("the background is restored %d times after a lipgloss reset, want 1: %q", n, line)
	}
	if !strings.HasSuffix(line, ansi.ResetStyle) {
		t.Errorf("the line does not close with a reset: %q", line)
	}
}

// TestSelectedLineCarriesTheBackgroundPastAChromaReset covers the other reset
// the drawing can produce. chroma does not use the ansi package and writes
// "\x1b[0m" after every token it colours, so a highlighted diff line is full
// of them.
func TestSelectedLineCarriesTheBackgroundPastAChromaReset(t *testing.T) {
	dark(t)

	line := theme.SelectedLine("a\x1b[0mb")
	bg := openingBackground(t, line)

	if !strings.Contains(line, "\x1b[0m"+bg) {
		t.Errorf("the background is not restored after a chroma reset: %q", line)
	}
}

// TestSelectedLineKeepsASpanWithItsOwnBackground guards the one case where the
// row's fill must *not* win: a GitHub label is drawn in the colour GitHub gave
// it, and the row's background belongs on either side of the chip, not over it.
func TestSelectedLineKeepsASpanWithItsOwnBackground(t *testing.T) {
	dark(t)

	chip := theme.Badge("d73a4a").Render(" bug ")
	line := theme.SelectedLine("title " + chip)
	bg := openingBackground(t, line)

	if !strings.Contains(line, "48;2;215;58;74") {
		t.Errorf("the label lost the colour GitHub gave it: %q", line)
	}
	if !strings.Contains(line, ansi.ResetStyle+bg) {
		t.Errorf("the row's background does not come back after the label: %q", line)
	}
}

// TestSelectedLineFollowsTheBackground guards the light variant of the
// selection colour: openingBackground reads whatever SelectedLine opens
// with rather than naming a colour, so a mistyped light hex would still pass
// the other tests above.
func TestSelectedLineFollowsTheBackground(t *testing.T) {
	dark(t)

	theme.SetDark(true)
	onDark := theme.SelectedLine("x")
	theme.SetDark(false)
	onLight := theme.SelectedLine("x")

	if onDark == onLight {
		t.Errorf("the same colour is used on both backgrounds: %q", onDark)
	}
}

// TestDiffLineCarriesTheBackgroundPastAChromaReset is the same guarantee
// SelectedLine has, for the second colour that now uses the same mechanism:
// a diff line is full of chroma's resets, and each one would end the tint.
func TestDiffLineCarriesTheBackgroundPastAChromaReset(t *testing.T) {
	dark(t)

	line := theme.DiffLine(domain.LineAdded, "a\x1b[0mb")
	bg := openingBackground(t, line)

	if !strings.Contains(line, "\x1b[0m"+bg) {
		t.Errorf("the background is not restored after a chroma reset: %q", line)
	}
	if !strings.HasSuffix(line, ansi.ResetStyle) {
		t.Errorf("the line does not close with a reset: %q", line)
	}
}

// TestDiffLineCarriesTheBackgroundPastALipglossReset covers the other reset
// the drawing produces: a line chroma has no lexer for still goes through
// lipgloss styles around it.
func TestDiffLineCarriesTheBackgroundPastALipglossReset(t *testing.T) {
	dark(t)

	line := theme.DiffLine(domain.LineRemoved, theme.Dim().Render("x")+"y")
	bg := openingBackground(t, line)

	if n := strings.Count(line, ansi.ResetStyle+bg); n != 1 {
		t.Errorf("the background is restored %d times after a lipgloss reset, want 1: %q", n, line)
	}
}

// TestDiffLineLeavesAContextLineAlone guards the third kind: an unchanged
// line is the majority of a diff and must stay as cheap and as plain as it
// is today.
func TestDiffLineLeavesAContextLineAlone(t *testing.T) {
	dark(t)

	if got := theme.DiffLine(domain.LineContext, "x"); got != "x" {
		t.Errorf("a context line was styled: %q", got)
	}
}

// TestDiffLineSeparatesAddedFromRemoved is what the whole change is for:
// the two states must not land on the same colour. Checked on both
// backgrounds, since dark and light each name their own pair of hexes and a
// typo in either pair would only show up on that one background.
func TestDiffLineSeparatesAddedFromRemoved(t *testing.T) {
	dark(t)

	for _, tc := range []struct {
		name string
		dark bool
	}{
		{"dark", true},
		{"light", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			theme.SetDark(tc.dark)
			added := theme.DiffLine(domain.LineAdded, "x")
			removed := theme.DiffLine(domain.LineRemoved, "x")
			if openingBackground(t, added) == openingBackground(t, removed) {
				t.Errorf("added and removed lines share a background: %q", added)
			}
		})
	}
}

// TestDiffLineFollowsTheBackground guards the light variants the way
// TestSelectedLineFollowsTheBackground guards the selection's: a mistyped
// light hex would pass every test above. Checked for both kinds, since added
// and removed each carry their own pair of hexes.
func TestDiffLineFollowsTheBackground(t *testing.T) {
	dark(t)

	for _, tc := range []struct {
		name string
		kind domain.DiffLineKind
	}{
		{"added", domain.LineAdded},
		{"removed", domain.LineRemoved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			theme.SetDark(true)
			onDark := theme.DiffLine(tc.kind, "x")
			theme.SetDark(false)
			onLight := theme.DiffLine(tc.kind, "x")

			if onDark == onLight {
				t.Errorf("the same colour is used on both backgrounds: %q", onDark)
			}
		})
	}
}
