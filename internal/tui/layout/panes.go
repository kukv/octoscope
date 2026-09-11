package layout

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/theme"
)

// JoinPanes puts the left pane beside the right one, padding whichever is
// shorter so the rule runs the full height of the taller one.
func JoinPanes(left, right []string, leftWidth int) []string {
	n := max(len(left), len(right))
	out := make([]string, n)
	for i := range n {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = padExact(l, leftWidth) + theme.Rule().Render("│") + r
	}
	return out
}

// padExact pads s out to exactly w columns with no ellipsis. A caller has
// already fitted its lines to leftWidth; running them through Pad here would
// reserve its usual one-column margin and clip a name that already fits.
func padExact(s string, w int) string {
	return s + strings.Repeat(" ", max(w-ansi.StringWidth(s), 0))
}
