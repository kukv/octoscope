package detail

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
)

func wheelDown() tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: 0, Y: 5, Button: tea.MouseWheelDown}
}

// longPR has a body taller than the viewport: a wheel test against a body
// that already fits proves nothing.
func longPR() domain.Item {
	return domain.Item{
		Ref:    domain.ItemRef{Kind: domain.ItemPR, Number: 1},
		Title:  "a long one",
		State:  domain.StateOpen,
		Body:   strings.Repeat("a paragraph of the description\n\n", 40),
		Change: &domain.Change{},
	}
}

func TestTheWheelScrollsTheBody(t *testing.T) {
	m := loaded(&fakeSource{pr: longPR()}, prRef())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	before := m.body.View()
	after, _ := m.Update(wheelDown())
	if after.body.View() == before {
		t.Error("the wheel did not scroll the body")
	}
}

// TestTheWheelIsIgnoredUnderAnOverlay keeps the wheel from scrolling text
// nobody can see. Each overlay is reached by the key that opens it.
func TestTheWheelIsIgnoredUnderAnOverlay(t *testing.T) {
	open := func(t *testing.T, k string, settle bool) Model {
		t.Helper()
		f := &fakeSource{
			pr:        longPR(),
			labels:    []domain.Label{{Name: "bug"}},
			reviewCtx: domain.ReviewContext{PullRequest: "PR_1"},
		}
		m := loaded(f, prRef())
		m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
		m, cmd := m.Update(key(k))
		if settle { // the overlay only opens once its own fetch answers
			m, _ = m.Update(cmd())
		}
		return m
	}

	for name, m := range map[string]Model{
		"the composer":     open(t, "c", false),
		"the confirmation": open(t, "x", false),
		"the picker":       open(t, "l", true),
		"the submit popup": open(t, "v", true),
	} {
		before := m.body.View()
		after, _ := m.Update(wheelDown())
		if after.body.View() != before {
			t.Errorf("the wheel scrolled the body under %s", name)
		}
	}
}
