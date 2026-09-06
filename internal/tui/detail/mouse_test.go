package detail

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

func wheelDown() tea.MouseWheelMsg {
	return tea.MouseWheelMsg{X: 0, Y: 5, Button: tea.MouseWheelDown}
}

// longPR has a body taller than the viewport, so there is something to
// scroll: a wheel test against a body that already fits proves nothing.
func longPR() gh.PR {
	return gh.PR{
		Number: 1, Title: "a long one", State: gh.StateOpen,
		Body: strings.Repeat("a paragraph of the description\n\n", 40),
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
// nobody can see: the composer, the confirmation and the picker are drawn
// over the body.
func TestTheWheelIsIgnoredUnderAnOverlay(t *testing.T) {
	base := loaded(&fakeSource{pr: longPR()}, prRef())
	base, _ = base.Update(tea.WindowSizeMsg{Width: 80, Height: 20})

	composing, confirming, picking, submitting := base, base, base, base
	composing.composing = true
	confirming.confirming = true
	picking.picking = true
	submitting.submitting = true

	for name, m := range map[string]Model{
		"the composer":     composing,
		"the confirmation": confirming,
		"the picker":       picking,
		"the submit popup": submitting,
	} {
		before := m.body.View()
		after, _ := m.Update(wheelDown())
		if after.body.View() != before {
			t.Errorf("the wheel scrolled the body under %s", name)
		}
	}
}
