package checks

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/golden"
	"github.com/kukv/octoscope/internal/i18n"
)

// goldenWidths are the widths a screen most commonly opens at.
var goldenWidths = []int{160, 120, 80}

var goldenLanguages = []struct {
	name string
	tag  language.Tag
}{
	{"en", language.English},
	{"ja", language.Japanese},
}

// goldenLog is the failing check's log, one line Japanese: a recording is
// the only thing that catches column drift in the log pane, and all-ASCII
// source would hide it.
func goldenLog() []gh.LogLine {
	return []gh.LogLine{
		{Step: "Run tests", Time: time.Date(2026, 9, 7, 10, 15, 30, 0, time.UTC), Text: "FAIL ./internal/gh (TestWalk)"},
		{Text: "深さの上限に達したら探索を打ち切る"},
	}
}

// goldenModel is the list alone, before any log has been asked for. The
// cursor opens on row 0, which arrange puts on the failing check ("sca").
func goldenModel(width int) Model {
	m := New(&fakeSource{checks: fixture(), log: goldenLog()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	return m
}

// checksLogModel is goldenModel with enter pressed, so the failing check's
// log -- goldenLog, Japanese line included -- is on screen.
func checksLogModel(width int) Model {
	m, cmd := goldenModel(width).Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	return m
}

// checksRerunModel is goldenModel with the rerun popup open on the same
// failing check.
func checksRerunModel(width int) Model {
	return press(goldenModel(width), "R")
}

// checksLoadingModel is the view before its fetch has landed.
func checksLoadingModel(width int) Model {
	m := New(&fakeSource{checks: fixture(), log: goldenLog()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	return m
}

// checksNoneModel is a pull request whose checks came back empty.
func checksNoneModel(width int) Model {
	m := New(&fakeSource{}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: gh.Checks{}})
	return m
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				golden.Assert(t, fmt.Sprintf("checks_%s_%d", lang.name, w), goldenModel(w).View())
				golden.Assert(t, fmt.Sprintf("checks_log_%s_%d", lang.name, w), checksLogModel(w).View())
				golden.Assert(t, fmt.Sprintf("checks_rerun_%s_%d", lang.name, w), checksRerunModel(w).View())
				golden.Assert(t, fmt.Sprintf("checks_loading_%s_%d", lang.name, w), checksLoadingModel(w).View())
				golden.Assert(t, fmt.Sprintf("checks_none_%s_%d", lang.name, w), checksNoneModel(w).View())
			})
		}
	}
}

// TestNothingOverrunsTheTerminal guards every golden state at every width:
// a row that overruns wraps, and everything under it draws a line lower
// than the layout believes.
// It does not run in parallel with the rest of the package: i18n.SetLanguage
// is global state, and a parallel test reading i18n.T while this one is
// mid-language-switch would race (internal/tui/diff's own golden test avoids
// the same way).
func TestNothingOverrunsTheTerminal(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			for i, line := range strings.Split(goldenModel(w).View(), "\n") {
				if got := ansi.StringWidth(line); got > w {
					t.Errorf("%s %d: line %d is %d columns:\n%s", lang.name, w, i, got, line)
				}
			}
		}
	}
}
