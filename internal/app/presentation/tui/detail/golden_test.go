package detail

import (
	"errors"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/app/domain"
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

func goldenPR() domain.PR {
	return domain.PR{
		Number: 12, Title: "レンダリングのパイプラインを置き換える",
		Author: domain.Author{Login: "kukv"}, State: domain.StateOpen,
		Review: domain.ReviewApproved, UpdatedAt: goldenAt,
		Body:   "This replaces the renderer.\n\n- one\n- two",
		Labels: []domain.Label{{Name: "enhancement", Color: "a2eeef"}},
		// Everything the meta pane can draw is filled in: a recording made
		// against an item with no branches, no checks and no assignee would
		// never show those rows at all.
		Assignees: []domain.Author{{Login: "alice"}},
		Checks:    domain.Checks{Total: 3, Passed: 1, Failed: 1, Running: 1},
		Head:      "feat/graph", Base: "main", Additions: 218, Deletions: 31,
		Comments: []domain.Comment{
			{Author: domain.Author{Login: "bob"}, Body: "見た目が良い", CreatedAt: goldenAt},
			// A second comment is what shows the bar starting over.
			{
				Author:    domain.Author{Login: "alice"},
				Body:      "ここは次の PR で直すつもり。\n\n長さのある段落をもう一つ置いて、折り返した行にも罫がつくことを記録に残す。",
				CreatedAt: goldenAt,
			},
		},
	}
}

// goldenMentionPR is the same item with the reader named in two places: the
// description and the first comment. The second comment names nobody, which
// is what makes the recording show the difference rather than only a colour.
func goldenMentionPR() domain.PR {
	pr := goldenPR()
	pr.Body = "This replaces the renderer.\n\n- one\n- two\n\ncc @kukv"
	pr.Comments[0].Body = "@kukv 見てもらえますか"
	return pr
}

// goldenRef names a repository, unlike prRef: the meta pane's first row is
// the reference, and an empty repository would leave it out of every
// recording.
func goldenRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12}
}

func goldenModel(width int) Model {
	// The order mirrors the app: the view is sized as soon as it is built,
	// and the item arrives afterwards, so the body is laid out for the width
	// it will be drawn at.
	f := &fakeSource{
		pr:        goldenPR(),
		labels:    []domain.Label{{Name: "bug", Color: "d73a4a"}},
		reviewCtx: domain.ReviewContext{PullRequest: "PR_128", Pending: "PRR_1"},
	}
	m := New(f, goldenRef())
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(fetch(f, goldenRef())())
	return m
}

// goldenMentionModel is goldenModel with the reader known, so the recording
// keeps what the highlight actually draws.
func goldenMentionModel(width int) Model {
	f := &fakeSource{pr: goldenMentionPR()}
	m := New(f, goldenRef()).SetViewer("kukv")
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(fetch(f, goldenRef())())
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
				golden.Assert(t, fmt.Sprintf("detail_mention_%s_%d", lang.name, w),
					goldenMentionModel(w).View())

				// Every state below is reached by the key that opens it:
				// a recording of an unreachable state guards nothing.
				confirming, _ := m.Update(key("x"))
				golden.Assert(t, fmt.Sprintf("detail_confirm_%s_%d", lang.name, w), confirming.View())

				composing, _ := m.Update(key("c"))
				golden.Assert(t, fmt.Sprintf("detail_compose_%s_%d", lang.name, w), composing.View())

				opening, cmd := m.Update(key("l"))
				golden.Assert(t, fmt.Sprintf("detail_picker_loading_%s_%d", lang.name, w), opening.View())
				picking, _ := opening.Update(cmd())
				golden.Assert(t, fmt.Sprintf("detail_picker_%s_%d", lang.name, w), picking.View())

				openingReview, cmd := m.Update(key("v"))
				submitting, _ := openingReview.Update(cmd())
				golden.Assert(t, fmt.Sprintf("detail_submit_%s_%d", lang.name, w), submitting.View())

				loading := New(&fakeSource{pr: goldenPR()}, goldenRef())
				loading, _ = loading.Update(tea.WindowSizeMsg{Width: w, Height: 40})
				golden.Assert(t, fmt.Sprintf("detail_loading_%s_%d", lang.name, w), loading.View())

				declined, _ := loading.Update(key("c"))
				golden.Assert(t, fmt.Sprintf("detail_loading_declined_%s_%d", lang.name, w), declined.View())

				// No other recording covers the error line.
				failed, _ := m.Update(stateErrorMsg{ref: goldenRef(), err: errors.New("boom")})
				golden.Assert(t, fmt.Sprintf("detail_error_%s_%d", lang.name, w), failed.View())
			})
		}
	}
}
