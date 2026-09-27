package repo

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
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
	f := &fakeSource{seed: []domain.RepoCandidate{
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

// Seeding can offer a hundred repositories per account. The dialog and the
// key bar under it have to stay on a terminal of forty lines, esc included,
// or there is no visible way back out.
func TestASeededDialogFitsTheTerminal(t *testing.T) {
	var seed []domain.RepoCandidate
	for i := range 100 {
		seed = append(seed, domain.RepoCandidate{Name: fmt.Sprintf("kukv/repo-%03d", i)})
	}
	m := sized(New(&fakeSource{seed: seed}, Options{}), 120)
	m, _ = m.SetCurrent("")
	m, cmd := m.Update(key("g"))
	for _, msg := range drain(t, cmd) {
		m, _ = m.Update(msg)
	}
	view := m.View()
	if got := strings.Count(view, "\n") + 1; got > 40 {
		t.Errorf("the dialog and its key bar take %d lines, want at most 40", got)
	}
	if !strings.Contains(view, "esc") {
		t.Errorf("the key bar is not on screen:\n%s", view)
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
	f := &fakeSource{seed: []domain.RepoCandidate{{Name: "kukv/octoscope"}}}
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
