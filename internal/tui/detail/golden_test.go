package detail

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/golden"
	"github.com/kukv/octoscope/internal/i18n"
)

var goldenWidths = []int{160, 120, 80}

var goldenLanguages = []struct {
	name string
	tag  language.Tag
}{
	{"en", language.English},
	{"ja", language.Japanese},
}

// goldenAt is a constant because the detail view prints absolute timestamps:
// a recording made against the wall clock would go stale immediately.
var goldenAt = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func goldenPR() gh.PR {
	return gh.PR{
		Number: 12, Title: "レンダリングのパイプラインを置き換える",
		Author: gh.Author{Login: "kukv"}, State: gh.StateOpen,
		Review: gh.ReviewApproved, UpdatedAt: goldenAt,
		Body:   "This replaces the renderer.\n\n- one\n- two",
		Labels: []gh.Label{{Name: "enhancement", Color: "a2eeef"}},
		Comments: []gh.Comment{
			{Author: gh.Author{Login: "bob"}, Body: "見た目が良い", CreatedAt: goldenAt},
		},
	}
}

func goldenModel(width int) Model {
	// The order mirrors the app: the view is sized as soon as it is built,
	// and the item arrives afterwards, so the body is laid out for the width
	// it will be drawn at.
	f := &fakeSource{
		pr:        goldenPR(),
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_128", PendingID: "PRR_1"},
	}
	m := New(f, prRef())
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(fetch(f, prRef())())
	return m
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })

				m := goldenModel(w)
				golden.Assert(t, fmt.Sprintf("detail_%s_%d", lang.name, w), m.View())

				confirming := m
				confirming.confirming = true
				golden.Assert(t, fmt.Sprintf("detail_confirm_%s_%d", lang.name, w), confirming.View())

				opening, cmd := m.Update(key("v"))
				submitting, _ := opening.Update(cmd())
				golden.Assert(t, fmt.Sprintf("detail_submit_%s_%d", lang.name, w), submitting.View())
			})
		}
	}
}
