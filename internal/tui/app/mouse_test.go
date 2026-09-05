package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

func click(x, y int) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

// loadedApp is a root whose children have their data: the first size is what
// starts the fetches, so the command it returns has to be run.
func loadedApp(t *testing.T, src Source, opts Options) Model {
	t.Helper()
	next, cmd := New(src, opts).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return resolve(t, next.(Model), cmd)
}

// tokenAt finds where a token is drawn on the whole screen, which is the
// coordinate space a mouse message arrives in.
func tokenAt(t *testing.T, m Model, token string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(m.View().Content, "\n") {
		s := ansi.Strip(line)
		if i := strings.Index(s, token); i >= 0 {
			return ansi.StringWidth(s[:i]), row
		}
	}
	t.Fatalf("%q is not on the screen:\n%s", token, ansi.Strip(m.View().Content))
	return 0, 0
}

// TestTheViewAsksForMouseEvents is the one line without which none of the
// handling below is ever reached.
func TestTheViewAsksForMouseEvents(t *testing.T) {
	if got := newTestModel(Options{}).View().MouseMode; got != tea.MouseModeCellMotion {
		t.Errorf("MouseMode = %v, want MouseModeCellMotion", got)
	}
}

func TestClickingATabSwitchesToIt(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		m := newTestModel(Options{HasRepo: true})

		x, _ := tokenAt(t, m, i18n.T("tab.repos"))
		next, _ := m.Update(click(x, 0))
		if next.(Model).tab != tabRepos {
			t.Errorf("lang %s: clicking the Repos tab did not switch to it", lang)
		}
		back, _ := next.(Model).Update(click(0, 0))
		if back.(Model).tab != tabWork {
			t.Errorf("lang %s: clicking the Work tab did not switch back", lang)
		}
	}
}

// TestClickingTheGapBetweenTabsDoesNothing covers the two spaces the row is
// joined with: a hit-test that divided the row evenly would land on a tab.
func TestClickingTheGapBetweenTabsDoesNothing(t *testing.T) {
	m := newTestModel(Options{HasRepo: true})
	gap := ansi.StringWidth("1 " + i18n.T("tab.work"))

	next, _ := m.Update(click(gap, 0))
	if next.(Model).tab != tabWork {
		t.Error("clicking between the tabs switched tabs")
	}
}

// TestAClickReachesTheActiveTabAtItsOwnCoordinates is the seam the two views
// meet at: the root draws a tab row above the child, and a child that was
// handed the screen's own row numbers would select the wrong card.
func TestAClickReachesTheActiveTabAtItsOwnCoordinates(t *testing.T) {
	src := &fakeSource{work: gh.Work{gh.SectionReviewRequested: {
		{Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 1}, Title: "first card"},
		{Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 2}, Title: "second card"},
	}}}
	m := loadedApp(t, src, Options{})

	x, y := tokenAt(t, m, "second card")
	next, _ := m.Update(click(x, y))
	ref, ok := next.(Model).work.SelectedRef()
	if !ok {
		t.Fatal("the click selected nothing")
	}
	if ref.Number != 2 {
		t.Errorf("the click selected #%d, want #2: the row was not translated", ref.Number)
	}
}

// TestAClickIsNotBroadcast keeps the two lists apart: a click meant for the
// board must not move the repository list's cursor as well.
func TestAClickIsNotBroadcast(t *testing.T) {
	src := &fakeSource{
		work: gh.Work{gh.SectionReviewRequested: {
			{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}, Title: "a card"},
			{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 2}, Title: "another card"},
		}},
		prs: []gh.PR{{Number: 10, Title: "first pr"}, {Number: 11, Title: "second pr"}},
	}
	m := loadedApp(t, src, Options{HasRepo: true})

	before, _ := m.repo.SelectedRef()
	x, y := tokenAt(t, m, "another card")
	next, _ := m.Update(click(x, y))
	if after, _ := next.(Model).repo.SelectedRef(); after != before {
		t.Errorf("a click on the board moved the repository list to %+v", after)
	}
}

// TestTheWheelReachesTheActiveTab covers the other half of the translation:
// the wheel is forwarded with the same row shift a click gets.
func TestTheWheelReachesTheActiveTab(t *testing.T) {
	src := &fakeSource{work: gh.Work{gh.SectionReviewRequested: {
		{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}, Title: "a card"},
		{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 2}, Title: "another card"},
	}}}
	m := loadedApp(t, src, Options{})

	_, y := tokenAt(t, m, "a card")
	next, _ := m.Update(tea.MouseWheelMsg{X: 2, Y: y, Button: tea.MouseWheelDown})
	if ref, _ := next.(Model).work.SelectedRef(); ref.Number != 2 {
		t.Errorf("the wheel selected #%d, want #2", ref.Number)
	}
}

// TestADragOrAReleaseIsDropped keeps every child from carrying a case for a
// message none of them acts on.
func TestADragOrAReleaseIsDropped(t *testing.T) {
	src := &fakeSource{work: gh.Work{gh.SectionReviewRequested: {
		{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}, Title: "a card"},
		{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 2}, Title: "another card"},
	}}}
	m := loadedApp(t, src, Options{})
	x, y := tokenAt(t, m, "another card")

	for name, msg := range map[string]tea.MouseMsg{
		"a release": tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft},
		"a drag":    tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft},
	} {
		next, cmd := m.Update(msg)
		if ref, _ := next.(Model).work.SelectedRef(); ref.Number != 1 {
			t.Errorf("%s moved the cursor to #%d", name, ref.Number)
		}
		if cmd != nil {
			t.Errorf("%s produced a command", name)
		}
	}
}

// TestAClickAboveTheBoardIsDropped covers the blank line between the tab row
// and the tab: it belongs to neither.
func TestAClickAboveTheBoardIsDropped(t *testing.T) {
	m := newTestModel(Options{HasRepo: true})
	if _, cmd := m.Update(click(0, 1)); cmd != nil {
		t.Error("a click on the blank line under the tab row produced a command")
	}
}

// TestTheErrorScreenIgnoresTheMouse matches what it does with keys: only q
// and esc get through, and neither is a click.
func TestTheErrorScreenIgnoresTheMouse(t *testing.T) {
	m := newTestModel(Options{HasRepo: true})
	failed, _ := m.fail(errors.New("gh: HTTP 502"))

	next, cmd := failed.(Model).Update(click(0, 0))
	if cmd != nil {
		t.Error("a click on the error screen produced a command")
	}
	if next.(Model).tab != tabWork {
		t.Error("a click on the error screen switched tabs")
	}
}
