package repo

import (
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// A list the user has to build by hand starts empty, and an empty screen
// with no way forward is where a first run stops.
func TestTheEmptySidebarOffersToSeedItself(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	m, _ = m.SetCurrent("")
	if !strings.Contains(m.View(), i18n.T("repos.seed_hint")) {
		t.Errorf("the empty list offers no way forward:\n%s", m.View())
	}
}

// The candidates are offered, not added. One account has been measured at 45
// repositories, which would make the count query 45 aliases wide and leave x
// as the only way back out of a list nobody asked for.
func TestSeedingOpensTheDialogWithWhatWasFound(t *testing.T) {
	f := &fakeSource{seed: []gh.RepoCandidate{
		{Name: "kukv/octoscope"}, {Name: "kukv/koto"},
	}}
	m := sized(New(f, Options{}), 120)
	m, _ = m.SetCurrent("")
	m, cmd := m.Update(key("g"))
	for _, msg := range drain(t, cmd) {
		m, _ = m.Update(msg)
	}
	view := m.View()
	for _, want := range []string{"kukv/octoscope", "kukv/koto"} {
		if !strings.Contains(view, want) {
			t.Errorf("the dialog does not offer %s:\n%s", want, view)
		}
	}
	if slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want nothing added without the user saying so", m.rowNames())
	}
}

// The three calls behind seeding take over ten seconds together, and a
// dialog that opens blank reads as a failure.
func TestSeedingSaysWhileItIsStillRunning(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	m, _ = m.SetCurrent("")
	m, _ = m.Update(key("g"))
	if !strings.Contains(m.View(), i18n.T("dialog.searching")) {
		t.Errorf("nothing says the seeding is running:\n%s", m.View())
	}
}

// Picking one of the seeded candidates is the same gesture as picking a
// search result, and it has to end with the repository saved.
func TestPickingASeededCandidateAddsIt(t *testing.T) {
	f := &fakeSource{seed: []gh.RepoCandidate{{Name: "kukv/octoscope"}}}
	m := sized(New(f, Options{}), 120)
	m, _ = m.SetCurrent("")
	m, cmd := m.Update(key("g"))
	for _, msg := range drain(t, cmd) {
		m, _ = m.Update(msg)
	}
	m, _ = m.Update(key("tab")) // onto the first candidate
	_, cmd = m.Update(key("enter"))
	drain(t, cmd)
	if !slices.Contains(f.saved, "kukv/octoscope") {
		t.Errorf("saved = %v, want the picked candidate", f.saved)
	}
}
