package detail

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/review"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// typeInto sends one KeyPressMsg per rune, the way a user typing into the
// note composer would.
func typeInto(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// TestAFailedSubmitKeepsTheNoteAndTheChosenEvent mirrors the diff view's
// test of the same name: a submit that fails over the network must not cost
// the reviewer what they already wrote (.claude/rules/errors.md).
func TestAFailedSubmitKeepsTheNoteAndTheChosenEvent(t *testing.T) {
	f := &fakeSource{
		pr:        gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_1"},
	}
	m := loaded(f, prRef())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m, cmd := m.Update(key("v"))
	m, _ = m.Update(cmd())

	m, _ = m.Update(key("tab")) // comment -> approve
	m = typeInto(m, "looks good")
	m, cmd = m.Update(key("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s produced no submit command")
	}
	m, _ = m.Update(review.ErrorMsg{Err: errors.New("boom from github")})

	if m.mode != modeSubmit {
		t.Fatalf("mode = %v after a failed submit, want the popup to stay open", m.mode)
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
		t.Error("the error text is empty after a failed submit")
	}
}

// TestAStaleReviewContextIsDropped pins the ref guard on reviewContextMsg
// and reviewContextErrMsg: a context fetched for an item the user has since
// left must not open the popup here. If it did, ctrl+s would build a Target
// carrying the item the user left behind's PullRequestID while the screen
// shows a different one -- the worst failure in this phase, since the
// review would go to the wrong pull request.
func TestAStaleReviewContextIsDropped(t *testing.T) {
	f := &fakeSource{pr: gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen}}
	other := gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 999}

	t.Run("reviewContextMsg", func(t *testing.T) {
		m := loaded(f, prRef())
		m, _ = m.Update(reviewContextMsg{ref: other, ctx: gh.ReviewContext{PullRequestID: "PR_OTHER"}})
		if m.mode == modeSubmit {
			t.Error("the popup opened on a stale reviewContextMsg, want it to stay closed")
		}
		if m.submit.Active() {
			t.Error("the popup opened against a pull request the user has left")
		}
	})

	t.Run("reviewContextErrMsg", func(t *testing.T) {
		m := loaded(f, prRef())
		m, _ = m.Update(key("v")) // the fetch cmd is deliberately not run
		m, _ = m.Update(reviewContextErrMsg{ref: other, err: errors.New("boom")})
		if m.mode != modeSubmit || m.phase != phaseLoading {
			t.Errorf("mode/phase = %v/%v after a stale reviewContextErrMsg, want it still waiting for its own fetch",
				m.mode, m.phase)
		}
		if m.errText != "" {
			t.Errorf("errText = %q after a stale reviewContextErrMsg, want empty", m.errText)
		}
	})
}

// TestLeavingTheSubmitPopupTakesItsErrorWithIt is the picker's rule for the
// review popup: esc must not leave the failure printed under the body.
func TestLeavingTheSubmitPopupTakesItsErrorWithIt(t *testing.T) {
	f := &fakeSource{
		pr:        gh.PR{Number: 1, Title: "first pr", State: gh.StateOpen},
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_1"},
	}
	m := loaded(f, prRef())
	m, cmd := m.Update(key("v"))
	m, _ = m.Update(cmd())
	m, _ = m.Update(review.ErrorMsg{Err: errors.New("boom from github")})

	m, cmd = m.Update(key("esc"))
	m, _ = m.Update(cmd()) // review.CancelledMsg
	if strings.Contains(ansi.Strip(m.View()), "boom from github") {
		t.Errorf("the popup's error is still on the body after esc:\n%s", ansi.Strip(m.View()))
	}
}
