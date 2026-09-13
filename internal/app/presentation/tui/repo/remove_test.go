package repo

import (
	"slices"
	"testing"
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
