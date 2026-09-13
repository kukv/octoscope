package repo

import (
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/i18n"
)

// With no settings file and no --repo, the list is empty for as long as the
// repository lookup takes -- measured at over six seconds cold. Saying "no
// repositories yet" during it is an answer octoscope does not have.
func TestTheEmptySidebarWaitsForTheLookup(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	if strings.Contains(m.View(), i18n.T("repos.none")) {
		t.Errorf("the list answered before the lookup did:\n%s", m.View())
	}
}

func TestTheEmptySidebarSaysSoOnceTheLookupAnswers(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	m, _ = m.SetCurrent("")
	if !strings.Contains(m.View(), i18n.T("repos.none")) {
		t.Errorf("the list never answered:\n%s", m.View())
	}
}

// --repo settles the question before the first frame, so that run must not
// spin.
func TestTheListDoesNotWaitWhenTheRepositoryWasNamed(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{Current: "kukv/octoscope"}), 120)
	if strings.Contains(m.View(), i18n.T("common.loading")) &&
		!strings.Contains(m.View(), "kukv/octoscope") {
		t.Errorf("the list waited for a lookup that had already answered:\n%s", m.View())
	}
}
