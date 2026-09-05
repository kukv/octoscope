package repo

import (
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

func wheel(up bool) tea.MouseWheelMsg {
	button := tea.MouseWheelDown
	if up {
		button = tea.MouseWheelUp
	}
	return tea.MouseWheelMsg{X: 2, Y: listTop, Button: button}
}

// tokenAt finds where a token is actually drawn. The tests ask the rendered
// list where a row is rather than recomputing the layout, so a hit-test that
// has drifted from the drawing fails here.
func tokenAt(t *testing.T, m Model, token string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(m.View(), "\n") {
		s := ansi.Strip(line)
		if i := strings.Index(s, token); i >= 0 {
			return ansi.StringWidth(s[:i]), row
		}
	}
	t.Fatalf("%q is not in the list:\n%s", token, ansi.Strip(m.View()))
	return 0, 0
}

func TestClickingARowSelectsItAndClickingAgainOpensIt(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{80, 120, 160} {
			m := sized(loadedModel(&fakeSource{prs: samplePRs()}), width)
			x, y := tokenAt(t, m, "second pr") // the cursor starts on the first

			selected, cmd := m.Update(click(x, y))
			if cmd != nil {
				t.Errorf("lang %s width %d: the first click opened the row", lang, width)
			}
			ref, ok := selected.SelectedRef()
			if !ok || ref.Number != 2 {
				t.Fatalf("lang %s width %d: clicking selected %+v, want #2", lang, width, ref)
			}

			_, cmd = selected.Update(click(x, y))
			if cmd == nil {
				t.Fatalf("lang %s width %d: clicking the selected row did not open it", lang, width)
			}
			open, ok := cmd().(OpenDetailMsg)
			if !ok || open.Ref.Number != 2 {
				t.Errorf("lang %s width %d: got %v, want OpenDetailMsg for #2", lang, width, cmd())
			}
		}
	}
}

func TestClickingASubTabSwitchesToIt(t *testing.T) {
	m := sized(loadedModel(&fakeSource{prs: samplePRs()}), 120)
	x, y := tokenAt(t, m, i18n.T("list.tab_issues"))

	next, cmd := m.Update(click(x, y))
	if next.tab != tabIssues {
		t.Fatal("clicking the Issues sub-tab did not switch to it")
	}
	if cmd == nil {
		t.Fatal("switching to an unloaded sub-tab did not fetch it")
	}
	if _, ok := cmd().(issueListMsg); !ok {
		t.Errorf("got %T, want the issue list to be fetched", cmd())
	}

	back, _ := next.Update(click(0, y))
	if back.tab != tabPRs {
		t.Error("clicking the Pull Requests sub-tab did not switch back")
	}
}

func TestClickingOutsideTheRowsChangesNothing(t *testing.T) {
	m := sized(loadedModel(&fakeSource{prs: samplePRs(), issues: []gh.Issue{}}), 120)
	before, _ := m.SelectedRef()

	for name, point := range map[string][2]int{
		"the title":                    {0, 0},
		"the blank line":               {0, 1},
		"the gap between the sub-tabs": {ansi.StringWidth(i18n.T("list.tab_prs")), subTabRow},
		"below the last row":           {2, 30},
	} {
		after, cmd := m.Update(click(point[0], point[1]))
		if got, _ := after.SelectedRef(); got != before || after.tab != m.tab {
			t.Errorf("clicking %s changed the list", name)
		}
		if cmd != nil {
			t.Errorf("clicking %s produced a command", name)
		}
	}
}

func TestTheWheelMovesTheCursor(t *testing.T) {
	m := sized(loadedModel(&fakeSource{prs: samplePRs()}), 120)

	down, _ := m.Update(wheel(false))
	if got, _ := down.SelectedRef(); got.Number != 2 {
		t.Errorf("scrolling down selected #%d, want #2", got.Number)
	}
	up, _ := down.Update(wheel(true))
	if got, _ := up.SelectedRef(); got.Number != 1 {
		t.Errorf("scrolling back up selected #%d, want #1", got.Number)
	}
	// The wheel must stop at the ends rather than wrap.
	for range 5 {
		up, _ = up.Update(wheel(true))
	}
	if got, _ := up.SelectedRef(); got.Number != 1 {
		t.Errorf("scrolling past the top selected #%d, want #1", got.Number)
	}
}
