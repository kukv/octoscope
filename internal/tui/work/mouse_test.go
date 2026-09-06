package work

import (
	"fmt"
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

func wheel(x, y int, up bool) tea.MouseWheelMsg {
	button := tea.MouseWheelDown
	if up {
		button = tea.MouseWheelUp
	}
	return tea.MouseWheelMsg{X: x, Y: y, Button: button}
}

// titleAt finds where a token is actually drawn, in display columns and
// lines. Asking the rendered board where a card is, rather than recomputing
// the layout, is what makes these tests catch a hit-test that has drifted
// away from the drawing.
func titleAt(t *testing.T, m Model, token string) (x, y int) {
	t.Helper()
	for row, line := range strings.Split(m.View(), "\n") {
		s := ansi.Strip(line)
		if i := strings.Index(s, token); i >= 0 {
			return ansi.StringWidth(s[:i]), row
		}
	}
	t.Fatalf("%q is not on the board:\n%s", token, ansi.Strip(m.View()))
	return 0, 0
}

// TestClickingACardSelectsIt is the test that closes the loop between the
// drawing and the hit-test: it clicks where the board actually drew a card,
// at every width and in both scripts, and asks which card is selected.
func TestClickingACardSelectsIt(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{80, 120, 160} {
			m := New(&fakeSource{work: alignedWork()})
			m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
			m, _ = m.Update(workMsg(alignedWork()))

			for i := range m.columns() {
				token := fmt.Sprintf("title-%d", i)
				x, y := titleAt(t, m, token)

				clicked, _ := m.Update(click(x, y))
				ref, ok := clicked.SelectedRef()
				if !ok {
					t.Fatalf("lang %s width %d: clicking %q selected nothing", lang, width, token)
				}
				if want := fmt.Sprintf("repo-%d", i); ref.Repo != want {
					t.Errorf("lang %s width %d: clicking %q selected %s, want %s",
						lang, width, token, ref.Repo, want)
				}
			}
		}
	}
}

// TestClickingTheSelectedCardOpensIt is the substitute for a double click,
// which Bubble Tea does not report (spec 4).
func TestClickingTheSelectedCardOpensIt(t *testing.T) {
	m := loaded() // the cursor starts on #12; "bump deps" is #3
	x, y := titleAt(t, m, "bump deps")

	first, cmd := m.Update(click(x, y))
	if cmd != nil {
		t.Error("the first click on a card opened it instead of selecting it")
	}
	_, cmd = first.Update(click(x, y))
	if cmd == nil {
		t.Fatal("clicking the selected card did not open it")
	}
	open, ok := cmd().(OpenDetailMsg)
	if !ok {
		t.Fatalf("got %T, want OpenDetailMsg", cmd())
	}
	if open.Ref.Number != 3 {
		t.Errorf("opened #%d, want #3", open.Ref.Number)
	}
}

func TestClickingAHeadingOrTheGapSelectsNothing(t *testing.T) {
	m := press(loaded(), "j") // second card of the first column
	before, _ := m.SelectedRef()

	colW := m.columnWidth(m.columns())
	for name, point := range map[string][2]int{
		"the heading":             {2, 0},
		"the gap between columns": {colW, 2},
		"below the last card":     {2, 39},
	} {
		after, cmd := m.Update(click(point[0], point[1]))
		got, _ := after.SelectedRef()
		if got != before {
			t.Errorf("clicking %s moved the cursor to %+v", name, got)
		}
		if cmd != nil {
			t.Errorf("clicking %s produced a command", name)
		}
	}
}

// TestOnlyTheLeftButtonSelects keeps a right click from moving the cursor
// under a context menu the terminal is drawing.
func TestOnlyTheLeftButtonSelects(t *testing.T) {
	m := loaded()
	x, y := titleAt(t, m, "bump deps")

	msg := click(x, y)
	msg.Button = tea.MouseRight
	after, _ := m.Update(msg)
	if got, _ := after.SelectedRef(); got.Number != 12 {
		t.Errorf("a right click moved the cursor to #%d", got.Number)
	}
}

func TestTheWheelMovesTheCursor(t *testing.T) {
	m := loaded()

	down, _ := m.Update(wheel(2, 2, false))
	if got, _ := down.SelectedRef(); got.Number != 3 {
		t.Errorf("scrolling down selected #%d, want #3", got.Number)
	}
	up, _ := down.Update(wheel(2, 2, true))
	if got, _ := up.SelectedRef(); got.Number != 12 {
		t.Errorf("scrolling back up selected #%d, want #12", got.Number)
	}
}

// TestTheWheelFollowsTheColumnUnderThePointer is why the wheel reads x at
// all: scrolling over a column is about that column, not about whichever one
// the keyboard last left the cursor in.
func TestTheWheelFollowsTheColumnUnderThePointer(t *testing.T) {
	m := loaded()
	x, y := titleAt(t, m, "an issue") // the Assigned column

	after, _ := m.Update(wheel(x, y, false))
	ref, ok := after.SelectedRef()
	if !ok {
		t.Fatal("the wheel left nothing selected")
	}
	if ref.Kind != gh.ItemIssue || ref.Number != 7 {
		t.Errorf("the wheel selected %+v, want the issue in the column under it", ref)
	}
}

// TestASingleColumnBoardStillHitTests covers the regime where View puts two
// extra lines above the board (spec 4.6): a hit-test that ignored them would
// be off by one card everywhere.
func TestASingleColumnBoardStillHitTests(t *testing.T) {
	m := loaded()
	m, _ = m.Update(tea.WindowSizeMsg{Width: 50, Height: 40})
	if m.boardTop() == 0 {
		t.Fatal("the board is not paged at 50 columns; this test covers nothing")
	}

	x, y := titleAt(t, m, "bump deps")
	after, _ := m.Update(click(x, y))
	if got, _ := after.SelectedRef(); got.Number != 3 {
		t.Errorf("clicking the second card selected #%d, want #3", got.Number)
	}
}
