// Package layout fits rendered text to the terminal.
package layout

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// ClipLines cuts every line of s to w display columns, marking a cut line with
// an ellipsis. Japanese takes two columns per character, so the count is never
// a byte or a rune count. A w of zero or less means the caller has no width
// yet — before the first tea.WindowSizeMsg — and s is returned unchanged.
func ClipLines(s string, w int) string {
	if w <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, w, "…")
	}
	return strings.Join(lines, "\n")
}

// FitKeyBar joins hints in order and drops from the low-priority end (the
// tail of the slice) until the joined line fits width. No ellipsis: a bar
// that shows fewer hints cleanly beats one that shows more but cuts one off
// mid-word. hints[0] is never dropped, so as long as the caller orders its
// most important hint first, that hint is what survives when only one hint
// fits — clipped, if even it does not fit, rather than left to overrun width.
// A width of zero or less means no width is known yet, and every hint is
// joined unclipped.
func FitKeyBar(hints []string, width int) string {
	if width <= 0 {
		return strings.Join(hints, "  ")
	}
	for n := len(hints); n > 0; n-- {
		joined := strings.Join(hints[:n], "  ")
		if ansi.StringWidth(joined) <= width {
			return joined
		}
	}
	return ClipLines(hints[0], width)
}
