// Package theme holds every colour the views draw with, in one place, so
// that a state has the same colour wherever it appears.
//
// A terminal's background cannot be assumed. Bubble Tea asks the terminal for
// it at start-up and reports the answer as a tea.BackgroundColorMsg; the root
// model passes that on to SetDark. Until it arrives the palette assumes a dark
// background, which is what most terminals have.
package theme

import (
	"image/color"
	"strconv"
	"sync"

	"charm.land/lipgloss/v2"

	"github.com/kukv/octoscope/internal/gh"
)

var (
	mu        sync.RWMutex
	lightDark = lipgloss.LightDark(true)
)

// SetDark tells the palette which way the terminal's background goes.
func SetDark(dark bool) {
	mu.Lock()
	defer mu.Unlock()
	lightDark = lipgloss.LightDark(dark)
}

// pick chooses between the two variants of one colour. The hex pairs below
// are GitHub's own light and dark values for the states they name, so a state
// reads the same here as it does in the web UI.
func pick(light, dark string) color.Color {
	mu.RLock()
	defer mu.RUnlock()
	return lightDark(lipgloss.Color(light), lipgloss.Color(dark))
}

func fg(light, dark string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(pick(light, dark))
}

// The palette. Success, danger, attention, accent and muted are the five roles
// every state below maps onto; nothing outside this file names a hex value.
func success() lipgloss.Style   { return fg("#1a7f37", "#3fb950") }
func danger() lipgloss.Style    { return fg("#cf222e", "#f85149") }
func attention() lipgloss.Style { return fg("#9a6700", "#d29922") }
func accent() lipgloss.Style    { return fg("#0969da", "#58a6ff") }
func muted() lipgloss.Style     { return fg("#57606a", "#8b949e") }

// Heading styles a column or section heading. The mockup letter-spaces and
// upper-cases them; a terminal cannot letter-space, and upper-casing does
// nothing to Japanese, so the heading is set apart by weight instead.
func Heading() lipgloss.Style { return muted().Bold(true) }

// Rule styles the lines that divide the screen: the vertical rules between
// board columns and the horizontal one above the drawer.
func Rule() lipgloss.Style { return fg("#d9dee4", "#262d39") }

// Card is the box one Work card is drawn in. The selected one takes the
// accent border and a filled background, which is what marks the selection
// once a card has a box of its own.
func Card(selected bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if selected {
		return s.BorderForeground(pick("#0e6f78", "#5bb4f5")).
			Background(pick("#e8eef5", "#1d2735"))
	}
	return s.BorderForeground(pick("#d9dee4", "#262d39"))
}

// Selected styles a selected row in a list that has no box to fill.
func Selected() lipgloss.Style {
	return lipgloss.NewStyle().Background(pick("#e8eef5", "#1d2735"))
}

// Count styles the tally beside a column heading. A column that wants
// attention says so in its own colour rather than only by being long.
func Count(attention bool) lipgloss.Style {
	if attention {
		return danger()
	}
	return muted()
}

// Dim styles text that is present but secondary: repository names, hints,
// footers, the empty-column note.
func Dim() lipgloss.Style { return muted() }

// Cursor styles the row or card under the selection.
func Cursor() lipgloss.Style { return accent().Bold(true) }

// ActiveTab and InactiveTab style the tab row.
func ActiveTab() lipgloss.Style   { return accent().Bold(true).Underline(true) }
func InactiveTab() lipgloss.Style { return muted() }

// Title styles the name of the thing on screen.
func Title() lipgloss.Style { return lipgloss.NewStyle().Bold(true) }

// Error styles a failure.
func Error() lipgloss.Style { return danger() }

// Accent styles a branch name or anything else the eye should land on inside
// a line of otherwise muted metadata.
func Accent() lipgloss.Style { return accent() }

// Added and Removed style the two halves of a diff's size.
func Added() lipgloss.Style   { return success() }
func Removed() lipgloss.Style { return danger() }

// Review styles the marker for a pull request's review state. A draft is
// muted whatever its review says, because nobody is being asked to look yet.
func Review(s gh.ReviewState, draft bool) lipgloss.Style {
	if draft {
		return muted()
	}
	switch s {
	case gh.ReviewApproved:
		return success()
	case gh.ReviewChangesRequested:
		return danger()
	case gh.ReviewRequired:
		return attention()
	default:
		return muted()
	}
}

// Check styles the marker for a rolled-up check state.
func Check(s gh.CheckState) lipgloss.Style {
	switch s {
	case gh.CheckSuccess:
		return success()
	case gh.CheckFailure:
		return danger()
	case gh.CheckRunning, gh.CheckPending:
		return attention()
	default:
		return muted()
	}
}

// Badge fills a GitHub label with the colour GitHub gave it. hex is the six
// digits GitHub returns, with no leading "#"; an unusable value falls back to
// the muted foreground so the name is still readable.
func Badge(hex string) lipgloss.Style {
	r, g, b, ok := rgb(hex)
	if !ok {
		return muted()
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color("#" + hex)).
		Foreground(lipgloss.Color(textOn(r, g, b)))
}

func rgb(hex string) (r, g, b int, ok bool) {
	if len(hex) != 6 {
		return 0, 0, 0, false
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(v >> 16), int(v>>8) & 0xff, int(v) & 0xff, true
}

// textOn picks black or white text for a background by measuring it rather
// than by eye: GitHub label colours span the whole range, and a fixed choice
// is unreadable on half of them.
//
// The weights are the BT.601 luma coefficients, scaled by a thousand so the
// arithmetic stays in integers. The threshold is the midpoint, which is the
// same rule GitHub applies to its own label text.
func textOn(r, g, b int) string {
	if 299*r+587*g+114*b > 128_000 {
		return "#000000"
	}
	return "#ffffff"
}
