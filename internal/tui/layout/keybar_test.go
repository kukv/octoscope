package layout_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/layout"
)

// hints is a stand-in for a view's key bar, ordered most important first,
// with a mix of ascii and Japanese so both display-width paths are covered.
func hints() []string {
	return []string{"esc:back", "c:comment", "v:review", "j/k:line", "{/}:hunk", "戻る:esc"}
}

// TestFitKeyBarNeverDropsTheFirstHint is what would have caught the bug that
// motivated this function: adding a hint pushed a footer past 80 columns,
// and naive truncation cut off the hint that told the user how to leave the
// view. The first hint must survive at every width the terminal is
// guaranteed to have.
func TestFitKeyBarNeverDropsTheFirstHint(t *testing.T) {
	for _, w := range []int{160, 120, 80} {
		bar := layout.FitKeyBar(hints(), w)
		if !strings.Contains(bar, hints()[0]) {
			t.Errorf("key bar at %d columns has no first hint: %q", w, bar)
		}
	}
}

// TestFitKeyBarFitsAt80 guards the terminal-width guarantee this project
// makes: whatever hints survive, the bar itself never overruns 80 columns.
func TestFitKeyBarFitsAt80(t *testing.T) {
	if got := ansi.StringWidth(layout.FitKeyBar(hints(), 80)); got > 80 {
		t.Errorf("key bar is %d columns wide at 80", got)
	}
}

// TestFitKeyBarShowsOnlyTheFirstHintWhenNarrow is the other end of the
// fit-aware bar: when only one hint can fit, it is the first and nothing
// else -- no ellipsis, no partial hint.
func TestFitKeyBarShowsOnlyTheFirstHintWhenNarrow(t *testing.T) {
	const narrow = 10 // fits "esc:back" but not "esc:back  c:comment"
	bar := layout.FitKeyBar(hints(), narrow)
	if got := ansi.StringWidth(bar); got > narrow {
		t.Errorf("key bar is %d columns wide at %d: %q", got, narrow, bar)
	}
	if strings.TrimSpace(bar) != hints()[0] {
		t.Errorf("key bar at %d columns = %q, want only the first hint %q", narrow, bar, hints()[0])
	}
}

// TestFitKeyBarClipsTheFirstHintWhenNarrowerThanItself covers the last
// resort: below the first hint's own width, even hints[:1] does not fit. The
// first hint must never be dropped, but it may be clipped -- returning it
// unclipped would let the bar overrun the width budget.
func TestFitKeyBarClipsTheFirstHintWhenNarrowerThanItself(t *testing.T) {
	const narrower = 5 // "esc:back" is 8 columns wide
	bar := layout.FitKeyBar(hints(), narrower)
	if got := ansi.StringWidth(bar); got > narrower {
		t.Errorf("key bar is %d columns wide at %d: %q", got, narrower, bar)
	}
	if bar == "" {
		t.Error("the first hint was dropped instead of clipped")
	}
}

// TestFitKeyBarWithNoWidthYetJoinsEverything covers the width<=0 case: no
// tea.WindowSizeMsg has arrived yet, so there is nothing to fit against.
func TestFitKeyBarWithNoWidthYetJoinsEverything(t *testing.T) {
	got := layout.FitKeyBar(hints(), 0)
	want := strings.Join(hints(), "  ")
	if got != want {
		t.Errorf("FitKeyBar(hints, 0) = %q, want %q", got, want)
	}
}
