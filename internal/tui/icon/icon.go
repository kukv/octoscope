// Package icon picks the glyphs the views use for review and check state.
// Phase 1 uses Unicode symbols only; the Nerd Font variants come later.
package icon

import (
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

// Review returns the one-column marker for a pull request's review state.
func Review(s gh.ReviewState, draft bool) string {
	if draft {
		return "◌"
	}
	switch s {
	case gh.ReviewApproved:
		return "✓"
	case gh.ReviewChangesRequested:
		return "×"
	default:
		return "•"
	}
}

// Issue returns the one-column marker for an issue. Issues on the board are
// always open, so there is nothing to distinguish: the marker is there to
// keep issue and pull-request cards aligned in the same column.
func Issue() string { return "◇" }

// Check returns the one-column marker for a rolled-up check state.
func Check(s gh.CheckState) string {
	switch s {
	case gh.CheckSuccess:
		return "✓"
	case gh.CheckFailure:
		return "×"
	case gh.CheckRunning, gh.CheckPending:
		return "◍"
	default:
		return " "
	}
}

// BarWidth is how many cells a checks bar occupies. A Work card has about 30
// columns, so the bar has to stay narrow (spec §4.1, §6.4).
const BarWidth = 7

// ChecksBar draws the passed / total ratio as a fixed-width bar, in two
// pieces: what has passed and what has not. They are returned apart because
// the two halves are coloured differently, and a caller cannot split a single
// string back up once the glyphs are chosen. Both are empty when there are no
// checks, so callers can leave the field out instead of drawing an empty bar.
func ChecksBar(c gh.Checks) (done, rest string) {
	if c.Total == 0 {
		return "", ""
	}
	filled := c.Passed * BarWidth / c.Total
	if filled == 0 && c.Passed > 0 {
		filled = 1
	}
	return strings.Repeat("▰", filled), strings.Repeat("▱", BarWidth-filled)
}
