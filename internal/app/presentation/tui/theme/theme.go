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
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
)

var (
	mu        sync.RWMutex
	lightDark = lipgloss.LightDark(true)
	isDark    = true
)

// SetDark tells the palette which way the terminal's background goes.
func SetDark(dark bool) {
	mu.Lock()
	defer mu.Unlock()
	lightDark = lipgloss.LightDark(dark)
	isDark = dark
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

// selection is the background a cursor row or card is marked with.
func selection() color.Color { return pick("#e8eef5", "#1d2735") }

// diffAddedBg and diffRemovedBg are the backgrounds an added and a removed
// line are filled with. GitHub's own values are alpha tints over the page's
// canvas -- addition rgba(46,160,67,.15) and deletion rgba(248,81,73,.10) on
// dark -- and a terminal has no alpha, so the dark pair below is those tints
// flattened onto #0d1117. The light pair is Primer's own opaque values.
func diffAddedBg() color.Color   { return pick("#e6ffec", "#12261e") }
func diffRemovedBg() color.Color { return pick("#ffebe9", "#25171c") }

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
			Background(selection())
	}
	return s.BorderForeground(pick("#d9dee4", "#262d39"))
}

// chromaReset is the reset chroma's terminal formatter writes after each
// token it colours. chroma does not go through the ansi package, so this is
// the one reset in the drawing that is not ansi.ResetStyle.
const chromaReset = "\x1b[0m"

// fillLine draws s filled with bg the whole way across.
//
// A background cannot simply be wrapped around a line that already has colour
// in it. Every coloured span ends in a reset, and a reset clears the
// background along with the foreground, so the fill would stop at the first
// one and leave the rest invisible from there on. Putting the background
// back after each reset is what makes a filled row both fully filled and
// still readable as itself.
//
// The two resets below are the only ones the drawing produces: lipgloss
// renders through ansi.Style.Styled, which always closes a span with
// ansi.ResetStyle, and chroma writes chromaReset.
//
// s must already be padded to the width it should fill: a background lands
// only on columns that have a character in them. A caller marking one word
// inside a line, rather than a whole row, passes the word alone -- the
// padding requirement is for a row, not for every call.
//
// A filled row must not be passed to fillLine again: a background is not a
// reset, so the inner fill wins outright and the outer colour never reaches
// the text.
func fillLine(s string, bg color.Color) string {
	seq := ansi.Style{}.BackgroundColor(bg).String()
	s = strings.ReplaceAll(s, ansi.ResetStyle, ansi.ResetStyle+seq)
	s = strings.ReplaceAll(s, chromaReset, chromaReset+seq)
	return seq + s + ansi.ResetStyle
}

// SelectedLine draws s as the selected row.
func SelectedLine(s string) string { return fillLine(s, selection()) }

// DiffLine fills an added or a removed line with the colour that says which
// it is, so the two are told apart by the whole row rather than by the one
// marker column. A context line is the majority of a diff and is returned
// untouched.
func DiffLine(k domain.DiffLineKind, s string) string {
	switch k {
	case domain.LineAdded:
		return fillLine(s, diffAddedBg())
	case domain.LineRemoved:
		return fillLine(s, diffRemovedBg())
	default:
		return s
	}
}

// Popup styles the frame around a small window drawn over an existing view:
// the review submission box.
func Popup() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).
		BorderForeground(pick("#d9dee4", "#262d39"))
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
func Review(s domain.ReviewState, draft bool) lipgloss.Style {
	if draft {
		return muted()
	}
	switch s {
	case domain.ReviewApproved:
		return success()
	case domain.ReviewChangesRequested:
		return danger()
	default:
		// ReviewNone and ReviewRequired both mean nobody has reviewed it yet:
		// GitHub reports no decision for a repository that requires none, and
		// "required" for one that does. The difference is the repository's
		// settings, not anything the user can act on differently.
		return attention()
	}
}

// Issue styles the marker for an issue. GitHub draws an open issue green, and
// every issue the views list is open. It is kept apart from the review and
// check colours it happens to share a green with: "this is an issue" and
// "this passed" are different things to say, and one must not follow the
// other when a palette changes.
func Issue() lipgloss.Style { return success() }

// Check styles the marker for a rolled-up check state.
func Check(s domain.CheckState) lipgloss.Style {
	switch s {
	case domain.CheckSuccess:
		return success()
	case domain.CheckFailure:
		return danger()
	case domain.CheckRunning, domain.CheckPending:
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

// Diff colours. GitHub's own values for an added and a removed line, so a
// diff reads here the way it reads in the web UI.
func DiffAdded() lipgloss.Style   { return fg("#1a7f37", "#3fb950") }
func DiffRemoved() lipgloss.Style { return fg("#cf222e", "#f85149") }

// HunkHeader styles the @@ line. It is a signpost rather than code, so it
// takes the muted colour and a weight of its own.
func HunkHeader() lipgloss.Style { return muted().Bold(true) }

// LineNumber styles the two numbers down the left of the diff.
func LineNumber() lipgloss.Style { return muted() }

// Thread styles a review comment drawn under the line it is about. A comment
// the user has not submitted yet is marked apart from one everyone can see:
// the difference decides whether pressing X would throw it away.
func Thread(pending bool) lipgloss.Style {
	if pending {
		return attention()
	}
	return accent()
}

// chromaStyle names the syntax-highlighting palette for each background.
// A chroma style *is* a palette, so it belongs here rather than in a view
// (.claude/rules/tui.md).
//
// lipgloss.LightDark chooses between two *colours*, so it cannot choose
// between two style names. SetDark keeps the answer as a bool for this.
func chromaStyle() string {
	mu.RLock()
	defer mu.RUnlock()
	if isDark {
		return "github-dark"
	}
	return "github"
}

// lexerCache remembers which lexer a path resolved to. lexers.Match globs a
// path against every lexer chroma has registered, which measures at about
// 1.5ms -- and Highlight is called once per visible row, every frame, so
// that one call is nearly the whole cost of drawing the diff. A path's
// lexer never changes while the program runs.
//
// A path that matched nothing is remembered too, as a nil lexer: a miss
// costs more than a hit, not less, because Match only gives up after trying
// every pattern it has.
var lexerCache sync.Map // path -> chroma.Lexer, nil when none matches

func lexerFor(path string) chroma.Lexer {
	if v, ok := lexerCache.Load(path); ok {
		// A remembered miss is a nil interface, which this assertion
		// reports as not-ok; either way the answer is nil.
		l, _ := v.(chroma.Lexer)
		return l
	}
	l := lexers.Match(path)
	lexerCache.Store(path, l)
	return l
}

// highlightCacheMax is how many coloured lines are remembered before the
// lot is dropped. A line and its colours run to about a kilobyte together,
// so this is a few megabytes -- and 8192 is two hundred screenfuls, far
// more than scrolling or moving between files needs.
const highlightCacheMax = 8192

// highlightKey is what decides a coloured line: the palette the background
// chose, the lexer the path chose, and the text itself. The caller clips
// the text to the width it has, so a resize asks with a different key
// rather than getting a line that no longer fits.
type highlightKey struct {
	dark bool
	path string
	code string
}

// highlightCache remembers coloured lines. Colouring one costs about 130
// microseconds, of which chroma spends two thirds rebuilding its
// style-to-escape-sequence table -- work that does not depend on the line
// at all. View runs on every message, and scrolling a line leaves all but
// one row of the screen the same as the frame before, so the same lines
// are coloured again and again.
//
// When it fills it is dropped whole rather than evicted one at a time.
// What has to survive is the screenful just drawn, and that is the most
// recently added however the limit is reached; one frame after the drop
// pays full price, and the rest are cheap again.
var (
	highlightMu    sync.RWMutex
	highlightLines = map[highlightKey]string{}
)

// cachedHighlight answers from the cache, or colours the line with colour
// and remembers it.
func cachedHighlight(key highlightKey, colour func() string) string {
	highlightMu.RLock()
	line, ok := highlightLines[key]
	highlightMu.RUnlock()
	if ok {
		return line
	}

	line = colour()

	highlightMu.Lock()
	defer highlightMu.Unlock()
	if len(highlightLines) >= highlightCacheMax {
		highlightLines = map[highlightKey]string{}
	}
	highlightLines[key] = line
	return line
}

// Highlight colours one line of source, chosen by the file's name.
//
// It is one line at a time because a diff is all we have: a string or a
// comment that opens on an earlier line looks unterminated here and simply
// goes uncoloured. Fetching whole files to colour a diff would cost a request
// per file, which is more than the colour is worth.
//
// A file chroma has no lexer for, and any failure inside chroma, comes back
// unchanged: the diff is still readable without colour.
func Highlight(path, code string) string {
	mu.RLock()
	dark := isDark
	mu.RUnlock()

	return cachedHighlight(highlightKey{dark: dark, path: path, code: code}, func() string {
		return highlight(path, code)
	})
}

// highlight is Highlight without the cache in front of it: everything below
// here is what colouring one line actually costs.
func highlight(path, code string) string {
	lexer := lexerFor(path)
	if lexer == nil {
		return code
	}
	style := styles.Get(chromaStyle())
	formatter := formatters.Get("terminal256")
	if style == nil || formatter == nil {
		return code
	}
	iter, err := lexer.Tokenise(nil, code)
	if err != nil {
		return code
	}
	var buf strings.Builder
	if err := formatter.Format(&buf, style, iter); err != nil {
		return code
	}
	// Tokenise appends a newline for lexers configured with EnsureNL (Makefile,
	// TypeScript, Rust, C, C++, etc.), and a newline inside a line would break
	// every row drawn under it once the diff view lands.
	return strings.TrimRight(buf.String(), "\n")
}
