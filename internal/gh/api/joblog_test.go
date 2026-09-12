package api

import (
	"strings"
	"testing"
)

// The server that builds the zip strips characters it cannot put in a path.
// A composite action's job is named "<job> / <action>", so a job whose name
// has a slash would never match its own directory unless the slash goes.
func TestASlashInAJobNameIsNotInItsZipEntry(t *testing.T) {
	t.Parallel()

	if got := logFileName("build / test"); got != "build  test" {
		t.Errorf("logFileName = %q, want %q", got, "build  test")
	}
}

// The server trims the name before it names the directory. A job whose name
// was written with a space around it -- or one left behind by a stripped
// slash -- would otherwise be looked for under a name no entry carries.
func TestSpaceAroundAJobNameIsNotInItsZipEntry(t *testing.T) {
	t.Parallel()

	if got := logFileName(" build "); got != "build" {
		t.Errorf("logFileName = %q, want %q", got, "build")
	}
}

// A colon goes the same way: Windows cannot have one in a path.
func TestAColonInAJobNameIsNotInItsZipEntry(t *testing.T) {
	t.Parallel()

	if got := logFileName("build: linux"); got != "build linux" {
		t.Errorf("logFileName = %q, want %q", got, "build linux")
	}
}

// The server truncates at 90 UTF-16 code units, not 90 bytes and not 90 runes.
// A name of multi-byte characters is cut in a place neither of the other two
// counts would pick, and a name cut in the wrong place matches nothing.
func TestALongJobNameIsCutWhereTheServerCutsIt(t *testing.T) {
	t.Parallel()

	// Each of these is one rune, three bytes, and one UTF-16 code unit.
	name := strings.Repeat("あ", 100)
	got := logFileName(name)
	if got != strings.Repeat("あ", 90) {
		t.Errorf("logFileName cut to %d characters, want 90", len([]rune(got)))
	}
}

// An emoji is two UTF-16 code units, so 50 of them already exceed the limit
// even though they are 50 runes. Counting runes would leave the name uncut and
// matching nothing.
func TestAnEmojiCountsTwiceTowardTheLimit(t *testing.T) {
	t.Parallel()

	name := strings.Repeat("😅", 50)
	got := logFileName(name)
	if len([]rune(got)) != 45 {
		t.Errorf("logFileName kept %d runes, want 45", len([]rune(got)))
	}
}

// The runner writes ANSI colour codes into the log. Handing an escape
// character to the screen lets the log repaint the terminal, so every C0 and
// C1 control turns into caret notation -- which is what gh hands the cli
// backend, so both backends show the same text.
func TestAnEscapeInTheLogCannotReachTheScreen(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("\x1b[31mFAIL\x1b[0m"); got != "^[[31mFAIL^[[0m" {
		t.Errorf("sanitizeControls = %q", got)
	}
}

// U+009B is the C1 control that means what the two-character escape means, and
// a terminal acts on it the same way. Mapping it by the arithmetic the C0
// range uses would hand it straight back.
func TestTheC1FormOfTheEscapeIsNeutralisedToo(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("\u009b[31mFAIL"); got != "^[[31mFAIL" {
		t.Errorf("sanitizeControls = %q", got)
	}
}

// Delete is a control character Go names as one, and gh does not rewrite it.
// Rewriting it here would put a "^" on a screen the cli backend leaves clean.
func TestDeleteIsLeftAloneTheWayGhLeavesIt(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("a\u007fb"); got != "a\u007fb" {
		t.Errorf("sanitizeControls = %q", got)
	}
}

// A tab is how the runner lines up test output. Turning it into caret notation
// would break every table in a log.
func TestATabSurvivesSanitizing(t *testing.T) {
	t.Parallel()

	if got := sanitizeControls("ok\tgithub.com/kukv/octoscope\t0.4s"); got != "ok\tgithub.com/kukv/octoscope\t0.4s" {
		t.Errorf("sanitizeControls = %q", got)
	}
}
