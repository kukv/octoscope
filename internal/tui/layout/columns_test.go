package layout_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/layout"
)

func TestPadLeavesAColumnBetweenFields(t *testing.T) {
	t.Parallel()

	// A name that exactly fills the field would run into the next one, so
	// Pad clips to one column short of the width.
	got := layout.Pad("abcdefgh", 8)
	if ansi.StringWidth(got) != 8 {
		t.Errorf("Pad width = %d, want 8: %q", ansi.StringWidth(got), got)
	}
	if got[len(got)-1] != ' ' {
		t.Errorf("Pad = %q, want it to end in a space", got)
	}
}

func TestPadCountsJapaneseAsTwoColumns(t *testing.T) {
	t.Parallel()

	got := layout.Pad("あいうえお", 8)
	if ansi.StringWidth(got) != 8 {
		t.Errorf("Pad width = %d, want 8: %q", ansi.StringWidth(got), got)
	}
}

func TestRightEndsFlush(t *testing.T) {
	t.Parallel()

	got := layout.Right("2h", 8)
	if ansi.StringWidth(got) != 8 {
		t.Errorf("Right width = %d, want 8: %q", ansi.StringWidth(got), got)
	}
	if got[len(got)-1] != 'h' {
		t.Errorf("Right = %q, want it to end in the text", got)
	}
}

func TestClipMarksWhatItCut(t *testing.T) {
	t.Parallel()

	got := layout.Clip("a very long title indeed", 10)
	if ansi.StringWidth(got) > 10 {
		t.Errorf("Clip width = %d, want at most 10: %q", ansi.StringWidth(got), got)
	}
	if !strings.Contains(got, "…") {
		t.Errorf("Clip = %q, want an ellipsis where it cut", got)
	}
}
