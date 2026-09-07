package diff

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
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

	if m.mode != modeSubmit {
		t.Fatal("the popup closed on a failed submit, want it to stay open")
	}
	out := m.View()
	if !strings.Contains(ansi.Strip(out), "looks good") {
		t.Errorf("the note was lost after a failed submit:\n%s", ansi.Strip(out))
	}
	wantSelected := theme.Selected().Render(i18n.T("submit.approve"))
	if !strings.Contains(out, wantSelected) {
		t.Errorf("the chosen event (approve) was lost after a failed submit:\n%s", ansi.Strip(out))
	}
	if m.errText == "" {
		t.Error("errText is empty after a failed submit")
	}
}

// TestOpeningAnOverlayLeavesAFailedSubmitBehind pins what one error string
// costs. Cancelling the popup takes the failed submission off the screen but
// not off the model -- nothing outside the popup draws it -- so each key that
// opens an overlay has to clear it, or the next composer or discard prompt
// would carry a submission's failure that has nothing to do with it.
func TestOpeningAnOverlayLeavesAFailedSubmitBehind(t *testing.T) {
	const boom = "boom from github"
	cases := []struct {
		name string
		key  string
		want mode
	}{
		{name: "c opens the composer", key: "c", want: modeCompose},
		{name: "v reopens the popup", key: "v", want: modeSubmit},
		{name: "X asks about the pending review", key: "X", want: modeDiscard},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// The context carries a pending review, so X has something to
			// ask about rather than declining before the popup is reached.
			ctx := threadFixture()
			ctx.PendingID = "PRR_9"
			m := loaded(t, 120, 40)
			m, _ = m.Update(reviewMsg{ref: m.ref, ctx: ctx})
			m = press(m, "v")
			m, _ = m.Update(review.ErrorMsg{Err: errors.New(boom)})
			if !strings.Contains(ansi.Strip(m.View()), boom) {
				t.Fatalf("precondition: the failed submit is not under the popup:\n%s", ansi.Strip(m.View()))
			}
			m, cmd := m.Update(keyPress("esc")) // the popup's own way out
			if cmd == nil {
				t.Fatal("esc produced no command to cancel the popup")
			}
			m, _ = m.Update(cmd())
			if m.mode != modeView {
				t.Fatalf("esc did not close the popup: mode = %v", m.mode)
			}

			m = cursorOnLine(t, m, gh.LineAdded, 13)
			m = press(m, tc.key)
			if m.mode != tc.want {
				t.Fatalf("%s did not open: mode = %v", tc.key, m.mode)
			}
			if strings.Contains(ansi.Strip(m.View()), boom) {
				t.Errorf("the failed submit came along into the overlay:\n%s", ansi.Strip(m.View()))
			}
		})
	}
}
