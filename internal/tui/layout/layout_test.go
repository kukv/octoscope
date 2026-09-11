package layout_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/layout"
)

func TestClipLines(t *testing.T) {
	cases := map[string]struct {
		in    string
		width int
		want  string
	}{
		"short line is untouched":      {"abc", 10, "abc"},
		"exact fit is untouched":       {"abcde", 5, "abcde"},
		"long line is cut":             {"abcdef", 5, "abcd…"},
		"every line is cut":            {"abcdef\nghijkl", 5, "abcd…\nghij…"},
		"a short line among long ones": {"abcdef\nx\nghijkl", 5, "abcd…\nx\nghij…"},
		// A Japanese character occupies two columns, so five columns hold two
		// of them plus the ellipsis, not four characters.
		"japanese counts two columns per character": {"あいうえお", 5, "あい…"},
		"no width leaves the text alone":            {"abcdef", 0, "abcdef"},
		"a negative width leaves the text alone":    {"abcdef", -1, "abcdef"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := layout.ClipLines(c.in, c.width); got != c.want {
				t.Errorf("ClipLines(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
			}
		})
	}
}

// TestClipLinesCountsColumnsNotBytes is the property the views rely on: after
// clipping, no line is wider than the terminal, whatever script it is in.
func TestClipLinesCountsColumnsNotBytes(t *testing.T) {
	const in = "レンダリングのパイプラインをまるごと置き換える refactor that nobody asked for"
	for _, w := range []int{1, 2, 3, 10, 50, 80} {
		for _, line := range strings.Split(layout.ClipLines(in, w), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: line is %d columns: %q", w, got, line)
			}
		}
	}
}

// TestClipLinesKeepsStyling checks that a clipped line keeps the escape
// sequence that colours it, so a cut label does not bleed its colour into the
// rest of the screen.
func TestClipLinesKeepsStyling(t *testing.T) {
	const in = "\x1b[31mabcdef\x1b[0m"
	got := layout.ClipLines(in, 4)
	if ansi.StringWidth(got) > 4 {
		t.Errorf("got %d columns, want at most 4: %q", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Errorf("the reset sequence was cut away: %q", got)
	}
}

// The notice sits on its own line: the ja key bar already fills 80 columns.
func TestNoticeFitsTheWidth(t *testing.T) {
	long := "gh api: gh: " + strings.Repeat("very long GitHub message ", 20)
	for _, w := range []int{80, 120, 160} {
		if got := ansi.StringWidth(layout.Notice(long, w)); got > w {
			t.Errorf("at %d columns the notice is %d wide", w, got)
		}
	}
}
