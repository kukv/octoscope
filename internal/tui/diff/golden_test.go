package diff

import (
	"fmt"
	"strings"
	"testing"

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

// goldenFixture adds a Japanese comment to the plain fixture, so a recording
// catches the column drift that all-ASCII source would hide.
func goldenFixture() []gh.FileDiff {
	files := fixture()
	files[0].Hunks[0].Lines = append([]gh.DiffLine{
		{Kind: gh.LineContext, OldLine: 11, NewLine: 11, Text: "\t// 深さの上限に達したら探索を打ち切る"},
	}, files[0].Hunks[0].Lines...)
	return files
}

func goldenModel(width int) Model {
	m := New(&fakeSource{files: goldenFixture()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(diffMsg{ref: m.ref, files: goldenFixture()})
	m = press(m, "j")
	return m
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				golden.Assert(t, fmt.Sprintf("diff_%s_%d", lang.name, w), goldenModel(w).View())
			})
		}
	}
}

// TestNoLineIsWiderThanTheTerminal is what catches a tab in a diff line
// pushing the cursor row past the right edge. goldenModel moves the cursor
// onto the row with the Japanese comment, which is also the row with a tab
// in front of it, so this exercises the exact row the golden files record.
// Every row is checked at every width, because a row that overruns wraps,
// and everything below it is then drawn a line lower than the layout
// believes.
func TestNoLineIsWiderThanTheTerminal(t *testing.T) {
	for _, w := range goldenWidths {
		for _, lang := range goldenLanguages {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				for i, line := range strings.Split(goldenModel(w).View(), "\n") {
					if got := ansi.StringWidth(line); got > w {
						t.Errorf("line %d is %d columns wide in a terminal %d wide: %q",
							i, got, w, ansi.Strip(line))
					}
				}
			})
		}
	}
}

func TestNoUnresolvedIDsInTheDiffView(t *testing.T) {
	for _, lang := range goldenLanguages {
		t.Run(lang.name, func(t *testing.T) {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			loading := New(&fakeSource{}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 1})
			loading, _ = loading.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			for name, view := range map[string]string{
				"loaded":  loaded(t, 120, 30).View(),
				"loading": loading.View(),
				"empty":   emptyDiff(t, 120, 30).View(),
			} {
				t.Run(name, func(t *testing.T) {
					i18n.AssertNoUnresolvedIDs(t, view)
				})
			}
		})
	}
}
