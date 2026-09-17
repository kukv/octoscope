package detail

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/usecase"
)

func withComments() usecase.Item {
	it := fullPRItem()
	it.Body = "This replaces the renderer.\n\n- one\n- two"
	it.Comments = []domain.Comment{
		{Author: domain.Author{Login: "bob"}, Body: "見た目が良い", CreatedAt: metaAt()},
		// The login is long enough to push the header past a narrow pane.
		// glamour wraps the body it is handed, so the header — which the view
		// builds itself — is the line that can run off the edge.
		{
			Author:    domain.Author{Login: "octoscope-release-bot"},
			Body:      "LGTM\n\nand a second paragraph",
			CreatedAt: metaAt(),
		},
	}
	return it
}

// TestEveryCommentLineCarriesTheBar is the whole point of the bar: a comment
// that runs past one line has to stay visibly one comment.
func TestEveryCommentLineCarriesTheBar(t *testing.T) {
	lines := commentLines(withComments().Comments[1], 40)

	if len(lines) < 3 {
		t.Fatalf("the comment came out as %d lines, want the header and two paragraphs", len(lines))
	}
	for _, l := range lines {
		if !strings.HasPrefix(ansi.Strip(l), icon.CommentBar()) {
			t.Errorf("line %q does not start with the comment bar", ansi.Strip(l))
		}
	}
}

// TestTheBodyDoesNotRepeatTheTitle covers the H1 that used to be generated:
// the title now has a line of its own above the panes.
func TestTheBodyDoesNotRepeatTheTitle(t *testing.T) {
	text := ansi.Strip(strings.Join(bodyLines(withComments(), 60), "\n"))

	if strings.Contains(text, "a pr") {
		t.Errorf("the body repeats the title:\n%s", text)
	}
}

func TestTheBodyNamesItsSections(t *testing.T) {
	text := ansi.Strip(strings.Join(bodyLines(withComments(), 60), "\n"))

	for _, want := range []string{"Description", "2 comments", "This replaces the renderer."} {
		if !strings.Contains(text, want) {
			t.Errorf("the body is missing %q:\n%s", want, text)
		}
	}
}

// TestAnItemWithNoCommentsHasNoCommentHeading keeps an empty heading off the
// screen.
func TestAnItemWithNoCommentsHasNoCommentHeading(t *testing.T) {
	it := fullPRItem()
	it.Body = "just the description"

	text := ansi.Strip(strings.Join(bodyLines(it, 60), "\n"))

	if strings.Contains(text, "comment") {
		t.Errorf("a heading was drawn for comments that do not exist:\n%s", text)
	}
}

func TestAnEmptyDescriptionSaysSo(t *testing.T) {
	it := fullPRItem()
	it.Body = "   "

	text := ansi.Strip(strings.Join(bodyLines(it, 60), "\n"))

	if !strings.Contains(text, "no description") {
		t.Errorf("an empty description was drawn as nothing:\n%s", text)
	}
}

// TestATableKeepsThePipesItWasWrittenWith covers what glamour does to a table
// that the reader did not ask for: it boxes the cells and stretches them to
// the full width, so three short columns become three wide ones.
func TestATableKeepsThePipesItWasWrittenWith(t *testing.T) {
	it := fullPRItem()
	it.Body = "before\n\n| 幅 | 列数 |\n|---|---|\n| 80 | 2 |\n\nafter"

	lines := markdownLines(it.Body, 70)
	text := ansi.Strip(strings.Join(lines, "\n"))

	for _, want := range []string{"| 幅 | 列数 |", "|---|---|", "| 80 | 2 |"} {
		if !strings.Contains(text, want) {
			t.Errorf("the table was redrawn rather than left alone; %q is missing:\n%s", want, text)
		}
	}
	// The prose around it still goes through glamour, and a blank line still
	// separates the three blocks.
	for _, want := range []string{"before", "after"} {
		if !strings.Contains(text, want) {
			t.Errorf("the prose around the table was lost; %q is missing:\n%s", want, text)
		}
	}
	if strings.Contains(text, "┼") {
		t.Errorf("the table was drawn as a box:\n%s", text)
	}
}

// TestAPipeOnItsOwnIsNotATable keeps ordinary prose out of the passthrough: a
// table is a row of cells with the dashes underneath, not any line with a pipe.
func TestAPipeOnItsOwnIsNotATable(t *testing.T) {
	src := "run `a | b` to pipe it"

	text := ansi.Strip(strings.Join(markdownLines(src, 70), "\n"))

	if !strings.Contains(text, "a | b") {
		t.Errorf("the line was mangled:\n%s", text)
	}
}

func TestTheBodyFitsItsWidth(t *testing.T) {
	for _, l := range bodyLines(withComments(), 40) {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line %q is %d columns wide, want at most 40", ansi.Strip(l), w)
		}
	}
}
