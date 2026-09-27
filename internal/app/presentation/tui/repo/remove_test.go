package repo

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestXRemovesTheRowAndSavesTheRest(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	m, _ = m.Update(key("h")) // focus the sidebar
	m, _ = m.Update(key("j")) // onto kukv/koto
	m, cmd := m.Update(key("x"))
	drain(t, cmd)
	if slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto gone", m.rowNames())
	}
	if !slices.Equal(f.saved, []string{"kukv/octoscope"}) {
		t.Errorf("saved = %v, want [kukv/octoscope]", f.saved)
	}
}

// Two saves in flight together run in whichever order their goroutines
// happen to, and the older list written last puts a removed row back. A
// second x waits for the first save and then writes the list as it stands.
func TestASecondSaveWaitsForTheFirst(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"a/1", "a/2", "a/3"}}), 120)
	m, _ = m.Update(key("h"))
	m, first := m.Update(key("x"))
	m, second := m.Update(key("x"))
	drain(t, second)
	if len(f.saved) != 0 {
		t.Fatalf("saved = %v before the first save answered, want nothing", f.saved)
	}

	for _, msg := range drain(t, first) {
		var cmd tea.Cmd
		m, cmd = m.Update(msg)
		drain(t, cmd)
	}
	if !slices.Equal(f.saved, []string{"a/3"}) {
		t.Errorf("saved last = %v, want [a/3]: the list as it stands", f.saved)
	}
}

// The repository the user is standing in is not in the settings file, so
// there is nothing to remove -- and taking it off the screen would lose the
// row they came to look at.
func TestXOnTheTemporaryRowDoesNothing(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}, Current: "kukv/koto"}), 120)
	m, _ = m.Update(key("h"))
	m, cmd := m.Update(key("x"))
	drain(t, cmd)
	if !slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want the temporary row kept", m.rowNames())
	}
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
}

// At eighty columns the sidebar is folded away and h cannot reach it, so an
// ungated x would remove a row that is not on screen.
func TestXDoesNothingWhileTheTableHasTheFocus(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	if m.focus != paneList {
		t.Fatalf("setup: the focus starts on %v, want the table", m.focus)
	}
	m, cmd := m.Update(key("x"))
	drain(t, cmd)
	if len(m.rowNames()) != 2 {
		t.Errorf("rows = %v, want both kept", m.rowNames())
	}
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
}

// The right pane must not keep showing a repository that has left the list.
func TestRemovingTheSelectedRowFetchesWhatIsNowUnderTheCursor(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j"))
	if m.selectedRepo() != "kukv/koto" {
		t.Fatalf("setup: selected %q, want kukv/koto", m.selectedRepo())
	}
	m, cmd := m.Update(key("x"))
	drain(t, cmd)
	if m.selectedRepo() != "kukv/octoscope" {
		t.Errorf("selected %q after removal, want kukv/octoscope", m.selectedRepo())
	}
	if !m.loading[m.tab] {
		t.Error("the new row's list was not fetched")
	}
}

// SetCurrent rebuilds the rows, and rebuilding them from the list as it was
// read at start-up would bring a removed repository back.
func TestRemovedRowsStayRemovedAcrossTheLookup(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope", "kukv/koto"}}), 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j"))
	m, cmd := m.Update(key("x"))
	drain(t, cmd)
	m, _ = m.SetCurrent("kukv/structure")
	if slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto still gone", m.rowNames())
	}
}
