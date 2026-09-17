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
	lines := commentLines(withComments().Comments[1], 40, "")

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
	text := ansi.Strip(strings.Join(bodyLines(withComments(), 60, ""), "\n"))

	if strings.Contains(text, "a pr") {
		t.Errorf("the body repeats the title:\n%s", text)
	}
}

func TestTheBodyNamesItsSections(t *testing.T) {
	text := ansi.Strip(strings.Join(bodyLines(withComments(), 60, ""), "\n"))

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

	text := ansi.Strip(strings.Join(bodyLines(it, 60, ""), "\n"))

	if strings.Contains(text, "comment") {
		t.Errorf("a heading was drawn for comments that do not exist:\n%s", text)
	}
}

func TestAnEmptyDescriptionSaysSo(t *testing.T) {
	it := fullPRItem()
	it.Body = "   "

	text := ansi.Strip(strings.Join(bodyLines(it, 60, ""), "\n"))

	if !strings.Contains(text, "no description") {
		t.Errorf("an empty description was drawn as nothing:\n%s", text)
	}
}

func TestTheBodyFitsItsWidth(t *testing.T) {
	for _, l := range bodyLines(withComments(), 40, "") {
		if w := ansi.StringWidth(l); w > 40 {
			t.Errorf("line %q is %d columns wide, want at most 40", ansi.Strip(l), w)
		}
	}
}

// TestOnlyTheCommentThatNamesTheReaderGetsTheHeavyBar is what the whole
// feature comes down to: two comments, one of them addressed at the reader,
// and the reader can tell which before reading either.
func TestOnlyTheCommentThatNamesTheReaderGetsTheHeavyBar(t *testing.T) {
	it := withComments()
	it.Comments[1].Body = "@kukv ここ見てもらえますか"

	for _, l := range commentLines(it.Comments[0], 40, "kukv") {
		if !strings.HasPrefix(ansi.Strip(l), icon.CommentBar()) {
			t.Errorf("a comment that names nobody starts with %q", ansi.Strip(l))
		}
	}
	for _, l := range commentLines(it.Comments[1], 40, "kukv") {
		if !strings.HasPrefix(ansi.Strip(l), icon.MentionBar()) {
			t.Errorf("a comment that names the reader starts with %q", ansi.Strip(l))
		}
	}
}

// TestTheHeavyBarCostsTheTextNoColumns guards the pane: the bar is what the
// body is indented by, and a bar that measured two columns would push one
// comment's text out of step with the next.
func TestTheHeavyBarCostsTheTextNoColumns(t *testing.T) {
	// The first comment is the one with the short author: the second one's
	// login is long enough to push its header past a narrow pane on its own,
	// which is what the pane's own Clip is for and not what this measures.
	it := withComments()
	it.Comments[0].Body = "@kukv ここ見てもらえますか"

	// The same comment drawn twice: once for the reader it names and once for
	// nobody. What the text is indented by has to come out the same both
	// times, and saying it this way leans on nothing glamour does.
	mine := ansi.Strip(commentLines(it.Comments[0], 40, "kukv")[0])
	plain := ansi.Strip(commentLines(it.Comments[0], 40, "")[0])

	if w := ansi.StringWidth(string([]rune(mine)[0])); w != 1 {
		t.Errorf("the heavy bar is %d columns wide, want 1", w)
	}
	if got, want := textColumn(mine), textColumn(plain); got != want {
		t.Errorf("a comment that names the reader starts its text at column %d, want %d", got, want)
	}
}

// textColumn is how far into the line the author's name sits: the width of
// everything the bar puts before it.
func textColumn(line string) int {
	return ansi.StringWidth(line[:strings.Index(line, "@")])
}

// TestTheDescriptionHeadingSaysWhenItNamesTheReader covers the other block:
// a description is not a comment and has no bar, so the heading above it is
// what carries the news.
//
// It sees that the words and the width did not move and that something in the
// escapes did, but not which colour moved nor that the rule beside the name
// was left alone -- a change that lit the whole rule would pass here. The
// detail_mention recordings are what guard that.
func TestTheDescriptionHeadingSaysWhenItNamesTheReader(t *testing.T) {
	it := withComments()
	it.Body = "cc @kukv お願いします"

	plain := bodyLines(withComments(), 60, "kukv")[0]
	mine := bodyLines(it, 60, "kukv")[0]

	if ansi.Strip(plain) != ansi.Strip(mine) {
		t.Fatalf("the two headings read differently: %q and %q",
			ansi.Strip(plain), ansi.Strip(mine))
	}
	if plain == mine {
		t.Error("the heading looks the same whether or not the description names the reader")
	}
}
