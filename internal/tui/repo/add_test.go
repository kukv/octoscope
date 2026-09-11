package repo

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

var errSaveFailed = errors.New("open config.yaml: permission denied")

func TestAOpensTheAddDialog(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{Repositories: []string{"kukv/octoscope"}}), 120)
	if strings.Contains(m.View(), i18n.T("dialog.add_repo_hint")) {
		t.Fatalf("setup: the dialog is already open:\n%s", m.View())
	}
	m, _ = m.Update(key("a"))
	if !strings.Contains(m.View(), i18n.T("dialog.add_repo_hint")) {
		t.Errorf("a did not open the dialog:\n%s", m.View())
	}
}

// The whole feature: the row appears and survives the next start-up.
func TestAddingARepositoryPutsItInTheListAndSavesIt(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/koto")
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	if !slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto among them", m.rowNames())
	}
	if !slices.Contains(f.saved, "kukv/koto") {
		t.Errorf("saved = %v, want kukv/koto among them", f.saved)
	}
}

// The repository the user is standing in leads the list without being in it,
// and a is the only thing that can write it there. Reading the name already
// on screen as a duplicate would leave no way to keep it.
func TestAddingPromotesTheTemporaryRow(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}, Current: "kukv/koto"}), 120)
	if slices.Contains(savedNames(m.rows), "kukv/koto") {
		t.Fatalf("setup: kukv/koto is already part of the saved list: %v", savedNames(m.rows))
	}
	// The dialog opens on that row's name, so enter is the whole gesture.
	m, _ = m.Update(key("a"))
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	if !slices.Contains(f.saved, "kukv/koto") {
		t.Errorf("saved = %v, want the temporary row written out", f.saved)
	}
	if n := slices.Index(m.rowNames(), "kukv/koto"); n < 0 {
		t.Errorf("rows = %v, want kukv/koto kept", m.rowNames())
	}
	if got := strings.Count(strings.Join(m.rowNames(), " "), "kukv/koto"); got != 1 {
		t.Errorf("rows = %v, want kukv/koto exactly once", m.rowNames())
	}
}

// Standing on the temporary row, a is almost always a request to keep that
// repository, so the field opens with it already typed in.
func TestTheDialogOpensOnTheTemporaryRowsName(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{Current: "kukv/koto"}), 120)
	m, _ = m.Update(key("a"))
	if m.dlg.Query() != "kukv/koto" {
		t.Errorf("the field holds %q, want the temporary row's name", m.dlg.Query())
	}
}

// Without --repo the repository lookup answers up to twenty seconds after
// start-up. Rebuilding the rows from the settings file's list as it was read
// then would undo anything added meanwhile.
func TestAddedRowsSurviveTheRepositoryLookup(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/koto")
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	m, _ = m.SetCurrent("kukv/structure")
	if !slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want kukv/koto kept across the lookup", m.rowNames())
	}
}

// The settings file is hand-editable, and a malformed entry there is already
// read as an uncountable row. Writing one on purpose would be worse.
func TestAddingRefusesANameThatIsNotOwnerSlashName(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "octoscope")
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
	if !strings.Contains(m.View(), i18n.T("dialog.invalid_name")) {
		t.Errorf("the dialog did not say why:\n%s", m.View())
	}
}

func TestAddingRefusesARepositoryAlreadyListed(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"kukv/octoscope"}}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/octoscope")
	m, cmd := m.Update(key("enter"))
	drain(t, cmd)
	if f.saved != nil {
		t.Errorf("saved %v, want nothing written", f.saved)
	}
	if !strings.Contains(m.View(), i18n.T("dialog.already_listed")) {
		t.Errorf("the dialog did not say why:\n%s", m.View())
	}
}

func TestEscapeClosesTheDialogWithoutAdding(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/koto")
	m, _ = m.Update(key("esc"))
	if strings.Contains(m.View(), i18n.T("dialog.add_repo_hint")) {
		t.Errorf("esc left the dialog open:\n%s", m.View())
	}
	if slices.Contains(m.rowNames(), "kukv/koto") {
		t.Errorf("rows = %v, want nothing added", m.rowNames())
	}
}

// One search has been measured at over a second, so a letter must schedule
// the search rather than run it.
func TestAKeystrokeOnlyStartsTheTimer(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	_, cmd := m.Update(key("l"))
	for _, msg := range drain(t, cmd) {
		if _, ok := msg.(candidatesMsg); ok {
			t.Error("a keystroke searched instead of scheduling one")
		}
	}
	if f.searches != 0 {
		t.Errorf("%d searches ran on one keystroke, want none", f.searches)
	}
}

// Once the pause has elapsed, exactly one search runs, for everything typed
// into the field rather than for the letter that happened to start the timer.
func TestTheTimerRunsTheSearch(t *testing.T) {
	f := &fakeSource{found: []gh.RepoCandidate{{Name: "charmbracelet/lipgloss", Stars: 9}}}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "lipgloss")
	m, cmd := m.Update(searchTickMsg{gen: m.searchGen})
	for _, msg := range drain(t, cmd) {
		m, _ = m.Update(msg)
	}
	if f.searches != 1 {
		t.Errorf("%d searches ran after the pause, want exactly one", f.searches)
	}
	if f.lastQuery != "lipgloss" {
		t.Errorf("searched for %q, want everything that was typed", f.lastQuery)
	}
	if !strings.Contains(m.View(), "charmbracelet/lipgloss") {
		t.Errorf("the suggestion never reached the dialog:\n%s", m.View())
	}
}

// Every keystroke schedules a tick, so all but the last arrive after the
// query has already moved on.
func TestAStaleTimerSearchesForNothing(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "lip")
	m, cmd := m.Update(searchTickMsg{gen: m.searchGen - 1})
	drain(t, cmd)
	if f.searches != 0 {
		t.Errorf("%d searches ran from a stale timer, want none", f.searches)
	}
}

// A list that reached the screen but not the disk would come back changed at
// the next start-up, so the failure has to be said out loud.
func TestASaveThatFailedSaysSo(t *testing.T) {
	f := &fakeSource{saveErr: errSaveFailed}
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(key("a"))
	m = typeInto(m, "kukv/koto")
	m, cmd := m.Update(key("enter"))
	for _, msg := range drain(t, cmd) {
		m, _ = m.Update(msg)
	}
	if !strings.Contains(m.View(), i18n.T("notice.save_failed")) {
		t.Errorf("nothing says the list was not written:\n%s", m.View())
	}
}
