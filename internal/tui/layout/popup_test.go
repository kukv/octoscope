package layout_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/tui/layout"
)

// TestPopupWidthAtZeroTerminalWidthIsAFallback covers the same gap dialog
// and search draw into before the first tea.WindowSizeMsg arrives: there is
// no terminal width yet, so PopupWidth must not return zero or negative.
func TestPopupWidthAtZeroTerminalWidthIsAFallback(t *testing.T) {
	t.Parallel()

	if got := layout.PopupWidth(0); got != 40 {
		t.Errorf("PopupWidth(0) = %d, want 40", got)
	}
}

// TestPopupWidthNeverGoesBelowTwenty is the floor: a narrower box would not
// hold a readable row.
func TestPopupWidthNeverGoesBelowTwenty(t *testing.T) {
	t.Parallel()

	if got := layout.PopupWidth(10); got != 20 {
		t.Errorf("PopupWidth(10) = %d, want 20", got)
	}
}

// TestPopupWidthNeverExceedsSixty is the ceiling: a wide terminal must not
// stretch the popup across it.
func TestPopupWidthNeverExceedsSixty(t *testing.T) {
	t.Parallel()

	if got := layout.PopupWidth(200); got != 60 {
		t.Errorf("PopupWidth(200) = %d, want 60", got)
	}
}

// TestPopupContentWidthSubtractsBorderAndPadding is what a line inside the
// box may occupy once theme.Popup's border and padding are taken out.
func TestPopupContentWidthSubtractsBorderAndPadding(t *testing.T) {
	t.Parallel()

	if got := layout.PopupContentWidth(46); got != 42 {
		t.Errorf("PopupContentWidth(46) = %d, want 42", got)
	}
}
