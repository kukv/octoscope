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
		if m.submitting {
			t.Error("submitting = true after a stale reviewContextMsg, want the popup to stay closed")
		}
		if m.submit.Active() {
			t.Error("the popup opened against a pull request the user has left")
		}
	})

	t.Run("reviewContextErrMsg", func(t *testing.T) {
		m := loaded(f, prRef())
		m.openingReview = true
		m, _ = m.Update(reviewContextErrMsg{ref: other, err: errors.New("boom")})
		if !m.openingReview {
			t.Error("openingReview = false after a stale reviewContextErrMsg, want it to still be waiting for its own fetch")
		}
		if m.actionErr != "" {
			t.Errorf("actionErr = %q after a stale reviewContextErrMsg, want empty", m.actionErr)
		}
	})
}
