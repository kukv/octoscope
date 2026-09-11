package layout

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Clip cuts s to w display columns. Japanese takes two columns per
// character, so the count is never a byte or a rune count.
func Clip(s string, w int) string { return ansi.Truncate(s, w, "…") }

// Pad clips s to one column short of w and pads it out, so two fields never
// run into each other.
func Pad(s string, w int) string {
	s = Clip(s, max(w-1, 0))
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}

// Right pads s on the left instead, so a column of ages ends flush.
func Right(s string, w int) string {
	s = Clip(s, w)
	return strings.Repeat(" ", max(w-ansi.StringWidth(s), 0)) + s
}
