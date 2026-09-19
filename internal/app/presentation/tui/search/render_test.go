package search

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
)

// sized is a model with results in it, at the width the test cares about.
func sized(t *testing.T, width int, items []domain.WorkItem) Model {
	t.Helper()

	m := New(&fakeSource{items: items})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m = next
	next, _ = m.Update(itemsMsg{gen: m.gen, items: items})
	return next
}

func TestTheQueryIsOnShowAboveTheFilters(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, nil)
	if !strings.Contains(m.View(), "is:open") {
		t.Errorf("the query the filters mean is not on screen:\n%s", m.View())
	}
}

func TestAResultRowShowsItsRepositoryAndNumber(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, []domain.WorkItem{{
		Ref:   domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 41},
		Title: "chore(deps): golangci-lint",
	}})
	view := m.View()
	// The repository column carries the name without its owner: every row
	// can come from a different one, and the owner is usually the same.
	for _, want := range []string{"octoscope", "#41", "chore(deps)"} {
		if !strings.Contains(view, want) {
			t.Errorf("the row does not carry %q:\n%s", want, view)
		}
	}
}

func TestEveryLineFitsTheTerminal(t *testing.T) {
	t.Parallel()

	items := make([]domain.WorkItem, 60)
	for i := range items {
		items[i] = domain.WorkItem{
			Ref:   domain.ItemRef{Repo: "kukv/a-repository-with-a-long-name", Number: i},
			Title: strings.Repeat("long title ", 20),
		}
	}
	for _, w := range []int{80, 120, 160} {
		m := sized(t, w, items)
		for i, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: line %d is %d columns:\n%s", w, i, got, line)
			}
		}
	}
}

// The terminal is 40 rows; the view must not draw more than that.
func TestTheViewFitsTheHeight(t *testing.T) {
	t.Parallel()

	items := make([]domain.WorkItem, 60)
	m := sized(t, 120, items)
	if got := len(strings.Split(m.View(), "\n")); got > 40 {
		t.Errorf("the view is %d rows, want at most 40", got)
	}
}

// TestTheKeyBarSitsOnTheLastRow is the complaint this change answers: the bar
// used to ride up with the results, so the hints moved every time a query
// found a different number of things. The board's bar has always been pinned.
func TestTheKeyBarSitsOnTheLastRow(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, 1, 3, 60} {
		m := sized(t, 120, make([]domain.WorkItem, n))
		if got := len(strings.Split(m.View(), "\n")); got != 40 {
			t.Errorf("%d results: the view is %d rows, want 40", n, got)
		}
	}
}

// Fifty is the cap the search asks for, so a full page may have been cut
// short. Saying "50" would claim there are exactly fifty.
func TestAFullPageSaysItMayHaveBeenCutShort(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, make([]domain.WorkItem, 50))
	if !strings.Contains(m.View(), "50+") {
		t.Errorf("a full page does not say it may be cut short:\n%s", m.View())
	}
}

// The capped count is one catalogue string, not a number glued to a word cut
// out of another one: a catalogue that put the unit first would silently lose
// it.
func TestTheCappedCountComesFromOneString(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, make([]domain.WorkItem, searchCap))
	view := m.View()
	if !strings.Contains(view, "50+") {
		t.Errorf("the capped count is missing:\n%s", view)
	}
	if strings.Contains(view, "50+  ") {
		t.Errorf("the capped count has a hole where a word was cut out:\n%s", view)
	}
}

// Under a hundred columns the filter pane folds away and the query and the
// results take the whole width.
func TestTheFilterPaneFoldsAwayWhenNarrow(t *testing.T) {
	t.Parallel()

	wide := sized(t, 120, nil).View()
	narrow := sized(t, 80, nil).View()
	if !strings.Contains(wide, "│") {
		t.Error("the wide view has no rule between the panes")
	}
	if strings.Contains(narrow, "│") {
		t.Errorf("the narrow view still draws two panes:\n%s", narrow)
	}
	if !strings.Contains(narrow, "is:open") {
		t.Errorf("the narrow view lost the query:\n%s", narrow)
	}
}

// The raw editor's own width follows the terminal, unlike the rest of the
// query row which is fixed to what was typed: a long query must not push it
// past the edge.
func TestTheRawEditorFitsTheTerminal(t *testing.T) {
	t.Parallel()

	for _, w := range []int{80, 120, 160} {
		m := sized(t, w, nil)
		m, _ = press(m, "e")
		if row, _, _ := strings.Cut(m.View(), "\n"); strings.Contains(row, "…") {
			t.Errorf("width %d: the raw editor's row is truncated before anything was typed:\n%s", w, row)
		}
		for _, r := range strings.Repeat("x", 200) {
			m, _ = press(m, string(r))
		}
		row, _, _ := strings.Cut(m.View(), "\n")
		if got := ansi.StringWidth(row); got > w {
			t.Errorf("width %d: the raw editor's row is %d columns:\n%s", w, got, row)
		}
	}
}

// The name prompt's row is the prompt text, a separating space, and the
// input field. The width given to the input field must leave room for that
// space, or the field claims a column past the edge and the row is clipped
// with a truncation mark even though nothing typed was too long to show.
func TestTheSaveNamePromptFitsTheTerminal(t *testing.T) {
	t.Parallel()

	for _, w := range []int{80, 120, 160} {
		m := sized(t, w, nil)
		m, _ = press(m, "s")
		row, _, _ := strings.Cut(m.View(), "\n")
		if got := ansi.StringWidth(row); got > w {
			t.Errorf("width %d: the name prompt row is %d columns:\n%s", w, got, row)
		}
		if strings.Contains(row, "…") {
			t.Errorf("width %d: the name prompt row is truncated with nothing typed:\n%s", w, row)
		}
	}
}

// The filter row is the filter's name in a fixed column and the field after
// it, clipped to the pane. The width given to the field must leave room for
// the cursor cell textinput draws past it, or the row is clipped with a
// truncation mark even though nothing typed was too long to show.
func TestATypedFilterIsNotTruncatedBeforeAnythingIsTyped(t *testing.T) {
	t.Parallel()

	for _, w := range []int{120, 160} {
		m := sized(t, w, nil)
		m, _ = press(m, "j") // type -> state
		m, _ = press(m, "j") // state -> org
		m, _ = press(m, "enter")
		for i, line := range strings.Split(m.View(), "\n") {
			if strings.Contains(line, "…") {
				t.Errorf("width %d: line %d is truncated with nothing typed:\n%s", w, i, line)
			}
		}
	}
}

// A typed filter's field is sized to the value column, so a long name
// scrolls inside it rather than pushing the result pane's rule out of line.
func TestATypedFilterFitsThePane(t *testing.T) {
	t.Parallel()

	for _, w := range []int{120, 160} {
		m := sized(t, w, nil)
		m, _ = press(m, "j") // type -> state
		m, _ = press(m, "j") // state -> org
		m, _ = press(m, "enter")
		for _, r := range strings.Repeat("x", 200) {
			m, _ = press(m, string(r))
		}
		for i, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("width %d: line %d is %d columns:\n%s", w, i, got, m.View())
			}
		}
	}
}

// A long filter value must be clipped to the pane whether or not the row is
// the one the cursor sits on: moving the cursor off must not un-clip it and
// let it overrun the divider between the panes.
func TestAFilterRowIsClippedWhetherOrNotItIsSelected(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, nil)
	m, _ = press(m, "j") // type -> state
	m, _ = press(m, "j") // state -> org
	m, _ = press(m, "enter")
	for _, r := range strings.Repeat("x", 30) {
		m, _ = press(m, string(r))
	}
	m, _ = press(m, "enter")

	if got := ansi.StringWidth(m.filterRow(FilterOrg)); got > filterPaneWidth {
		t.Errorf("selected filter row is %d columns, want at most %d:\n%s", got, filterPaneWidth, m.filterRow(FilterOrg))
	}

	m, _ = press(m, "j") // org -> repo, leaving org unselected
	if got := ansi.StringWidth(m.filterRow(FilterOrg)); got > filterPaneWidth {
		t.Errorf("unselected filter row is %d columns, want at most %d:\n%s", got, filterPaneWidth, m.filterRow(FilterOrg))
	}
}

// The popup draws one row per saved query. Nothing caps them, so a list
// grown over months pushes the box's top edge and the key bar off the
// screen -- the whole popup becomes unusable.
func TestThePickerFitsTheHeight(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, nil)
	qs := make([]domain.SavedQuery, 60)
	for i := range qs {
		qs[i] = domain.SavedQuery{Name: fmt.Sprintf("q%d", i), Query: "is:open"}
	}
	m = m.SetSavedQueries(qs)
	m, _ = press(m, "ctrl+o")
	if got := len(strings.Split(m.View(), "\n")); got > 40 {
		t.Errorf("the picker drew %d lines into a 40-row terminal", got)
	}
}

// The window that TestThePickerFitsTheHeight cuts to must still follow the
// cursor, or scrolling past the top of a long list would hide the entry the
// user is on.
func TestThePickerWindowFollowsTheCursor(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, nil)
	qs := make([]domain.SavedQuery, 60)
	for i := range qs {
		qs[i] = domain.SavedQuery{Name: fmt.Sprintf("q%d", i), Query: "is:open"}
	}
	m = m.SetSavedQueries(qs)
	m, _ = press(m, "ctrl+o")
	for range 59 {
		m, _ = press(m, "j")
	}
	if !strings.Contains(m.View(), "q59") {
		t.Errorf("the selected row scrolled out of view:\n%s", m.View())
	}
}
