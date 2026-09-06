// Package icon picks the glyphs the views use for review and check state.
//
// Three sets are kept, because a terminal is not guaranteed to be able to draw
// any given glyph: Nerd Font glyphs need a patched font installed, the Unicode
// symbols need a font with reasonable coverage, and ASCII needs nothing at all
// (spec 4.5).
package icon

import (
	"os"
	"strings"
	"sync"

	"github.com/kukv/octoscope/internal/gh"
)

// Set is one family of glyphs.
type Set int

const (
	// Unicode is the default: symbols any modern font is likely to have.
	Unicode Set = iota
	// Nerd uses the glyphs of a Nerd Font patched font.
	Nerd
	// ASCII is what is left when nothing else can be drawn.
	ASCII
)

// EnvVar names the environment variable that chooses a set. There is no
// reliable way to detect a patched font — a terminal reports neither the font
// in use nor its coverage, and TERM_PROGRAM says nothing about what the user
// installed — so the choice is the user's, and the default is the set that
// works without one.
const EnvVar = "OCTOSCOPE_ICONS"

// glyphs is one set's whole vocabulary. Every field is one display column
// wide; TestEveryGlyphIsOneColumn holds the sets to that.
type glyphs struct {
	approved, changesRequested, reviewPending, draft string
	issue                                            string
	checkSuccess, checkFailure, checkRunning         string
	barDone, barRest                                 string
	collapsed, commentBar                            string
}

var sets = map[Set]glyphs{
	Unicode: {
		approved: "✓", changesRequested: "×", reviewPending: "•", draft: "◌",
		issue:        "◇",
		checkSuccess: "✓", checkFailure: "×", checkRunning: "◍",
		barDone: "▰", barRest: "▱",
		collapsed: "▸", commentBar: "▌",
	},
	Nerd: {
		approved: "", changesRequested: "", reviewPending: "", draft: "",
		issue:        "",
		checkSuccess: "", checkFailure: "", checkRunning: "",
		barDone: "█", barRest: "░",
		collapsed: "▸", commentBar: "▌",
	},
	ASCII: {
		approved: "+", changesRequested: "x", reviewPending: "*", draft: "o",
		issue:        "o",
		checkSuccess: "+", checkFailure: "x", checkRunning: "~",
		barDone: "#", barRest: "-",
		collapsed: ">", commentBar: "|",
	},
}

var (
	mu      sync.RWMutex
	current = Unicode
)

// Use switches the set every glyph is drawn from.
func Use(s Set) {
	mu.Lock()
	defer mu.Unlock()
	current = s
}

func active() glyphs {
	mu.RLock()
	defer mu.RUnlock()
	return sets[current]
}

// Resolve picks the set from the --icons flag and the environment, in that
// order. "auto", the empty string and anything unrecognised all mean Unicode:
// a typo must not leave the board undrawable.
func Resolve(flag string) Set {
	for _, candidate := range []string{flag, os.Getenv(EnvVar)} {
		switch strings.ToLower(strings.TrimSpace(candidate)) {
		case "nerd":
			return Nerd
		case "ascii":
			return ASCII
		case "unicode":
			return Unicode
		}
	}
	return Unicode
}

// Review returns the one-column marker for a pull request's review state.
func Review(s gh.ReviewState, draft bool) string {
	g := active()
	if draft {
		return g.draft
	}
	switch s {
	case gh.ReviewApproved:
		return g.approved
	case gh.ReviewChangesRequested:
		return g.changesRequested
	default:
		return g.reviewPending
	}
}

// Issue returns the one-column marker for an issue. Issues on the board are
// always open, so there is nothing to distinguish: the marker is there to
// keep issue and pull-request cards aligned in the same column.
func Issue() string { return active().issue }

// Check returns the one-column marker for a rolled-up check state.
func Check(s gh.CheckState) string {
	g := active()
	switch s {
	case gh.CheckSuccess:
		return g.checkSuccess
	case gh.CheckFailure:
		return g.checkFailure
	case gh.CheckRunning, gh.CheckPending:
		return g.checkRunning
	default:
		return " "
	}
}

// Collapsed returns the one-column marker for a folded thread.
func Collapsed() string { return active().collapsed }

// CommentBar returns the one-column bar drawn down the left of a review
// comment.
func CommentBar() string { return active().commentBar }

// BarWidth is how many cells a checks bar occupies. A Work card has about 30
// columns, so the bar has to stay narrow (spec 4.1, 6.4).
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
	g := active()
	return strings.Repeat(g.barDone, filled), strings.Repeat(g.barRest, BarWidth-filled)
}
