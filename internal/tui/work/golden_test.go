package work

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/golden"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
)

// goldenWidths are the three regimes the board degrades through: four columns
// with a drawer, four columns without one, and a single paged column
// (spec 4.6).
var goldenWidths = []int{160, 120, 80}

var goldenLanguages = []struct {
	name string
	tag  language.Tag
}{
	{"en", language.English},
	{"ja", language.Japanese},
}

// goldenFetchedAt is the clock the recordings are made against. The cards show
// relative times, so the board's fetchedAt has to be a constant rather than
// the wall clock, or every recording would go stale a minute after it was made.
var goldenFetchedAt = time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)

// goldenModel is a board holding the overlong fixture, so that each recording
// shows a card that has to be truncated as well as ones that fit.
func goldenModel(width int) Model {
	m := New(&fakeSource{work: overlongWork()})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(workMsg(overlongWork()))
	m.fetchedAt = goldenFetchedAt
	return m
}

// TestGoldenIconSets records the board once per glyph set. The board is what
// the sets exist for, and a set that draws the wrong width shows up here as a
// column that no longer lines up (spec 4.5).
func TestGoldenIconSets(t *testing.T) {
	for name, set := range map[string]icon.Set{"nerd": icon.Nerd, "ascii": icon.ASCII} {
		t.Run(name, func(t *testing.T) {
			icon.Use(set)
			t.Cleanup(func() { icon.Use(icon.Unicode) })
			golden.Assert(t, "work_icons_"+name, goldenModel(120).View())
		})
	}
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				golden.Assert(t, fmt.Sprintf("work_%s_%d", lang.name, w), goldenModel(w).View())
			})
		}
	}
}
