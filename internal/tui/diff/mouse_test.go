package diff

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

// rowY finds the screen row a given diff row was actually drawn on, so the
// test clicks where the view put it rather than where the test guessed.
func rowY(t *testing.T, m Model, needle string) int {
	t.Helper()
	for y, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(ansi.Strip(line), needle) {
			return y
		}
	}
	t.Fatalf("%q was never drawn:\n%s", needle, ansi.Strip(m.View()))
	return 0
}

func click(m Model, x, y int) Model {
	m, _ = m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return m
}

func TestClickingTheDiffSelectsThatLine(t *testing.T) {
	m := loaded(t, 120, 30)
	y := rowY(t, m, "if depth <= 0 {")
	m = click(m, sidebarWidth+m.gutter()+5, y)
	if got := m.currentRow(); got.kind != rowLine || got.line.Text != expandTabs("\tif depth <= 0 {") {
		t.Errorf("the click selected %+v, want the line it landed on", got)
	}
}

func TestClickingTheFileListSelectsThatFile(t *testing.T) {
	m := loaded(t, 120, 30)
	y := rowY(t, m, "logo.png")
	m = click(m, 2, y)
	if m.file != 1 {
		t.Errorf("file = %d, want the one that was clicked", m.file)
	}
}

// TestClickingASettledThreadTwiceOpensIt: the first click selects, the second
// on an already-selected row opens. No double click -- Bubble Tea does not
// report one, and measuring the gap ourselves would put a clock in Update
// (spec 4.0).
func TestClickingASettledThreadTwiceOpensIt(t *testing.T) {
	m := withThreads(t, 120, 40)
	y := rowY(t, m, "settled comment")
	m = click(m, sidebarWidth+5, y)
	if strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Error("one click opened the thread")
	}
	m = click(m, sidebarWidth+5, y)
	if !strings.Contains(ansi.Strip(m.View()), "settled long ago") {
		t.Errorf("the second click did not open it:\n%s", ansi.Strip(m.View()))
	}
}

func TestTheWheelMovesWhateverIsUnderIt(t *testing.T) {
	m := loaded(t, 120, 30)
	before := m.row
	m, _ = m.Update(tea.MouseWheelMsg{X: sidebarWidth + 5, Y: 6, Button: tea.MouseWheelDown})
	if m.row == before {
		t.Error("the wheel over the diff moved nothing")
	}

	m2 := loaded(t, 120, 30)
	m2, _ = m2.Update(tea.MouseWheelMsg{X: 2, Y: 6, Button: tea.MouseWheelDown})
	if m2.file != 1 {
		t.Errorf("the wheel over the file list moved to file %d, want 1", m2.file)
	}
}

// TestTheWheelFollowsThePointerNotTheLastFocusedPane: moveRow redirects to
// moveFile whenever the sidebar is the focused pane, which is right for j/k
// but wrong for a wheel -- the pointer decides, not wherever the cursor last
// was left by a click.
func TestTheWheelFollowsThePointerNotTheLastFocusedPane(t *testing.T) {
	m := click(loaded(t, 120, 30), 2, 4) // select the sidebar, file 0 (many rows)
	if !m.sidebar {
		t.Fatal("the click did not focus the sidebar; this test covers nothing")
	}
	before := m.row
	m, _ = m.Update(tea.MouseWheelMsg{X: sidebarWidth + 5, Y: 6, Button: tea.MouseWheelDown})
	if m.row == before {
		t.Error("the wheel over the diff pane moved nothing, even though the sidebar was still focused")
	}
}

// TestClicksOutsideThePanesDoNothing: the header and the key bar are not
// clickable, and a click on them must not select whatever row the arithmetic
// happens to land on.
func TestClicksOutsideThePanesDoNothing(t *testing.T) {
	m := loaded(t, 120, 30)
	before := m.row
	m = click(m, 5, 0)  // the header
	m = click(m, 5, 29) // the key bar
	if m.row != before {
		t.Errorf("row moved to %d on a click outside the panes", m.row)
	}
}

// manyLineFixture is one file with more lines than a short terminal can show
// at once, so moving the selection past the bottom exercises the diff
// pane's own scroll (m.top).
func manyLineFixture() []gh.FileDiff {
	lines := make([]gh.DiffLine, 40)
	for i := range lines {
		lines[i] = gh.DiffLine{Kind: gh.LineAdded, NewLine: i + 1, Text: fmt.Sprintf("line-%02d", i)}
	}
	return []gh.FileDiff{
		{
			Path: "big.go", Status: gh.FileModified, Additions: 40,
			Hunks: []gh.Hunk{{Header: "@@ -0,0 +1,40 @@", Lines: lines}},
		},
	}
}

// TestClickingAScrolledDiffPaneSelectsTheRightRow proves the hit test reads
// m.top: without it, a click after the pane has scrolled would select
// whatever row happens to sit at that offset from the top of the file
// instead of the row actually drawn there.
func TestClickingAScrolledDiffPaneSelectsTheRightRow(t *testing.T) {
	files := manyLineFixture()
	m := New(&fakeSource{files: files}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 15})
	m, _ = m.Update(diffMsg{ref: m.ref, files: files})
	for range 30 {
		m = press(m, "j")
	}
	if m.top == 0 {
		t.Fatal("the pane did not scroll; this test covers nothing")
	}

	y := rowY(t, m, "line-25")
	clicked := click(m, sidebarWidth+m.gutter()+5, y)
	if got := clicked.currentRow(); got.kind != rowLine || !strings.Contains(got.line.Text, "line-25") {
		t.Errorf("clicking a scrolled row selected %+v, want line-25", got)
	}
}
