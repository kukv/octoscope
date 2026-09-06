package work

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
	"github.com/kukv/octoscope/internal/tui/icon"
)

// goldenWidths are the three regimes the board degrades through: four columns
// with boxed cards and a drawer, four plain columns without one, and a single
// paged column (spec 4.6).
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
func goldenModel(width int) Model { return goldenBoard(overlongWork(), width, 40) }

func goldenBoard(w gh.Work, width, height int) Model {
	m := New(&fakeSource{work: w})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(workMsg(w))
	m.fetchedAt = goldenFetchedAt
	return m
}

// tallWork is a column with more cards than any terminal can show at once.
// Without it every recording fits, and the recordings would say nothing about
// what happens when a column overflows — which is the ordinary case.
func tallWork() gh.Work {
	w := overlongWork()
	base := w[gh.SectionReviewRequested][0]
	for i := range 20 {
		card := base
		card.Ref = gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 100 + i}
		card.Title = fmt.Sprintf("chore(deps): bump dependency number %d", i)
		card.Labels = nil
		w[gh.SectionReviewRequested] = append(w[gh.SectionReviewRequested], card)
	}
	return w
}

// TestGoldenFitsTheTerminal records a board that overflows, at two heights.
// The whole screen has to fit: before this, a column with fifty cards pushed
// the drawer and the key bar off the bottom.
func TestGoldenFitsTheTerminal(t *testing.T) {
	for _, height := range []int{24, 40} {
		t.Run(fmt.Sprintf("h%d", height), func(t *testing.T) {
			m := goldenBoard(tallWork(), 120, height)
			out := m.View()

			if got := len(strings.Split(out, "\n")); got > height {
				t.Errorf("the board drew %d lines into a terminal %d high", got, height)
			}
			if !strings.Contains(ansi.Strip(out), ansi.Strip(m.keyBar())) {
				t.Errorf("the key bar was pushed off the screen:\n%s", ansi.Strip(out))
			}
			golden.Assert(t, fmt.Sprintf("work_tall_%d", height), out)
		})
	}
}

// TestScrollingFollowsTheCursorDownALongColumn is the other half: the board
// fits, so a cursor past the last visible card has to bring the window with
// it or the selection disappears.
func TestScrollingFollowsTheCursorDownALongColumn(t *testing.T) {
	m := goldenBoard(tallWork(), 120, 24)
	visible := m.visibleCards(m.boardHeight())
	if visible >= len(m.work[gh.SectionReviewRequested]) {
		t.Fatal("the fixture fits on screen; this test covers nothing")
	}

	for range visible + 2 {
		m = press(m, "j")
	}
	ref, ok := m.SelectedRef()
	if !ok {
		t.Fatal("nothing is selected")
	}
	if !strings.Contains(ansi.Strip(m.View()), m.work[m.section()][m.row].Title) {
		t.Errorf("the selected card %+v is not on screen:\n%s", ref, ansi.Strip(m.View()))
	}
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
