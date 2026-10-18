package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
)

// refetched presses r and hands over the diff half of its answer, so the
// screen draws again; the review half is left in flight. It answers the
// number that half was asked under.
func refetched(m Model) (Model, int) {
	m = press(m, "r")
	m, _ = m.Update(diffMsg{ref: m.ref, gen: m.diffGen, files: fixture()})
	return m, m.reviewGen
}

// staleAnswer is what an r pressed earlier comes back with: the review as it
// was before whatever the user has done since, with one comment in it that
// only that answer carries.
func staleAnswer(m Model, gen int) reviewMsg {
	ctx := pendingFixture()
	ctx.Pending = "PRR_9"
	ctx.Threads = append(ctx.Threads, domain.ReviewThread{
		Path: "graph/walk.go", Line: 14, Side: domain.SideRight,
		Comments: []domain.ThreadComment{{Author: domain.Author{Login: "kukv"}, Body: "from before", Pending: true}},
	})
	return reviewMsg{ref: m.ref, gen: gen, ctx: ctx}
}

// An r answered after the user has started something else describes the
// review before it: a discard's comments come back as not sent yet, a posted
// comment goes missing. Starting another operation cancels the refetch, and
// a cancelled refetch's answer is dropped when it lands.
func TestStartingAnotherOperationCancelsTheRefetch(t *testing.T) {
	for _, tt := range []struct {
		name string
		open func(t *testing.T, m Model) Model
		mode mode
	}{
		{"a line comment", func(t *testing.T, m Model) Model {
			return press(cursorOnLine(t, m, domain.LineAdded, 13), "c")
		}, modeCompose},
		{"a discard", func(_ *testing.T, m Model) Model { return press(m, "X") }, modeDiscard},
		{"a submit", func(_ *testing.T, m Model) Model { return press(m, "v") }, modeSubmit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := withPending(t)
			m.review.Pending = "PRR_9"
			m, gen := refetched(m)

			m = tt.open(t, m)
			if m.mode != tt.mode {
				t.Fatalf("mode = %v, want %v: the operation did not start", m.mode, tt.mode)
			}
			m, _ = m.Update(staleAnswer(m, gen))

			if out := ansi.Strip(m.View()); strings.Contains(out, "from before") {
				t.Errorf("the cancelled refetch's answer was drawn:\n%s", out)
			}
		})
	}
}

// With nothing started since, r's answer is the newest there is and is drawn.
func TestARefetchNothingCancelledIsDrawn(t *testing.T) {
	m, gen := refetched(withPending(t))
	m, _ = m.Update(staleAnswer(m, gen))

	if out := ansi.Strip(m.View()); !strings.Contains(out, "from before") {
		t.Errorf("r's answer was not drawn:\n%s", out)
	}
}
