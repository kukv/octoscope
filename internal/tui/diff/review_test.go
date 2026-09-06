package diff

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/review"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// TestAFailedSubmitKeepsTheNoteAndTheChosenEvent pins review.ErrorMsg's
// holder-side handling: a submit that fails over the network must not cost
// the reviewer what they already wrote (.claude/rules/errors.md). Only
// m.sending is cleared inside review.Model itself; the note, the chosen
// event and the popup staying open all depend on this holder leaving
// everything else alone.
func TestAFailedSubmitKeepsTheNoteAndTheChosenEvent(t *testing.T) {
	m := withThreads(t, 120, 40)
	m = press(m, "v")
	m, _ = m.Update(keyPress("tab")) // comment -> approve
	m = typeInto(m, "looks good")
	m, cmd := m.Update(keyPress("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s produced no submit command")
	}
	m, _ = m.Update(review.ErrorMsg{Err: errors.New("boom from github")})

	if !m.submitting {
		t.Fatal("submitting = false after a failed submit, want the popup to stay open")
	}
	out := m.View()
	if !strings.Contains(ansi.Strip(out), "looks good") {
		t.Errorf("the note was lost after a failed submit:\n%s", ansi.Strip(out))
	}
	wantSelected := theme.Selected().Render(i18n.T("submit.approve"))
	if !strings.Contains(out, wantSelected) {
		t.Errorf("the chosen event (approve) was lost after a failed submit:\n%s", ansi.Strip(out))
	}
	if m.submitErr == "" {
		t.Error("submitErr is empty after a failed submit")
	}
}
