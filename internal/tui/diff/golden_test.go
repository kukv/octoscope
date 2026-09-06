package diff

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/golden"
	"github.com/kukv/octoscope/internal/i18n"
)

// goldenWidths are the widths a screen most commonly opens at.
var goldenWidths = []int{160, 120, 80}

var goldenLanguages = []struct {
	name string
	tag  language.Tag
}{
	{"en", language.English},
	{"ja", language.Japanese},
}

// goldenFixture adds a Japanese comment to the plain fixture, so a recording
// catches the column drift that all-ASCII source would hide.
func goldenFixture() []gh.FileDiff {
	files := fixture()
	files[0].Hunks[0].Lines = append([]gh.DiffLine{
		{Kind: gh.LineContext, OldLine: 11, NewLine: 11, Text: "\t// 深さの上限に達したら探索を打ち切る"},
	}, files[0].Hunks[0].Lines...)
	return files
}

// goldenReview names the pull request, its branches, and one open thread
// with a Japanese comment, so the recording covers the header's title line
// and a thread row at once.
func goldenReview() gh.ReviewContext {
	return gh.ReviewContext{
		PullRequestID: "PR_128",
		Title:         "add relation graph traversal",
		Head:          "feat/graph",
		Base:          "main",
		Threads: []gh.ReviewThread{
			{
				Path: "graph/walk.go", Line: 13, Side: gh.SideRight,
				Comments: []gh.ThreadComment{
					{Author: gh.Author{Login: "kukv"}, Body: "ここは 2 が既定ではないでしょうか、直しておいてもらえますか?"},
				},
			},
		},
	}
}

func goldenModel(width int) Model {
	m := New(&fakeSource{files: goldenFixture()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(diffMsg{ref: m.ref, files: goldenFixture()})
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: goldenReview()})
	m = press(m, "j")
	return m
}

// composingModel is goldenModel with the composer open on the row the cursor
// already sits on (a rowLine, per goldenModel's own comment): a row this
// wide, with the composer's rows taken out of the pane's height budget, is
// what would first show a composer wide or tall enough to overrun the
// terminal.
func composingModel(width int) Model {
	return press(goldenModel(width), "c")
}

// postingModel is composingModel after ctrl+s, with the post cmd
// deliberately not run so the model is caught mid-send.
func postingModel(width int) Model {
	m := typeInto(composingModel(width), "why not 2?")
	m, _ = m.Update(keyPress("ctrl+s"))
	return m
}

// wideLineNumberFixture is a file whose hunk starts past line nine thousand,
// so its line numbers need five columns instead of four.
func wideLineNumberFixture() []gh.FileDiff {
	return []gh.FileDiff{
		{
			Path: "vendor/generated.go", Status: gh.FileModified, Additions: 2, Deletions: 1,
			Hunks: []gh.Hunk{
				{
					Header: "@@ -10240,3 +10240,4 @@ func generated() {",
					Lines: []gh.DiffLine{
						{Kind: gh.LineContext, OldLine: 10240, NewLine: 10240, Text: strings.Repeat("x", 200)},
						{Kind: gh.LineRemoved, OldLine: 10241, Text: strings.Repeat("x", 200)},
						{Kind: gh.LineAdded, NewLine: 10241, Text: strings.Repeat("x", 200)},
						{Kind: gh.LineAdded, NewLine: 10242, Text: strings.Repeat("x", 200)},
					},
				},
			},
		},
	}
}

// wideLineNumberModel leaves the cursor on the hunk header (row 0), never on
// one of the five-digit line rows: a cursor row goes through fit/clip, which
// truncates, and would hide the overrun this model exists to catch.
func wideLineNumberModel(width int) Model {
	m := New(&fakeSource{files: wideLineNumberFixture()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 130})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(diffMsg{ref: m.ref, files: wideLineNumberFixture()})
	return m
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				golden.Assert(t, fmt.Sprintf("diff_%s_%d", lang.name, w), goldenModel(w).View())
			})
		}
	}
}

// TestNoLineIsWiderThanTheTerminal is what catches a tab in a diff line
// pushing the cursor row past the right edge. goldenModel moves the cursor
// onto the row with the Japanese comment, which is also the row with a tab
// in front of it, so this exercises the exact row the golden files record.
// Every row is checked at every width, because a row that overruns wraps,
// and everything below it is then drawn a line lower than the layout
// believes.
// reviewFailureModel is goldenModel with a review-context failure on top,
// its message long and partly Japanese so a line this wide has to be
// truncated rather than allowed to overrun.
func reviewFailureModel(width int) Model {
	m := goldenModel(width)
	m, _ = m.Update(reviewErrMsg{
		ref: m.ref,
		err: errors.New("GraphQL: このプルリクエストのレビュー情報を取得できませんでした (fetchReviewContext)"),
	})
	return m
}

// submittingModel is goldenModel with the review popup open, a pending
// review already on the diff so the popup has a line comment to count.
func submittingModel(width int) Model {
	m := goldenModel(width)
	m.review.PendingID = "PRR_1"
	return press(m, "v")
}

// discardingModel is goldenModel with the discard confirmation up, over a
// pending review: X does nothing without one to discard.
func discardingModel(width int) Model {
	m := goldenModel(width)
	m.review.PendingID = "PRR_1"
	return press(m, "X")
}

func TestNoLineIsWiderThanTheTerminal(t *testing.T) {
	models := map[string]func(int) Model{
		"tab":              goldenModel,
		"wide_line_number": wideLineNumberModel,
		"review_failure":   reviewFailureModel,
		"composing":        composingModel,
		"posting":          postingModel,
		"submitting":       submittingModel,
		"discarding":       discardingModel,
	}
	// 99 and 100 straddle minWidthForSidebar, the one width where the layout
	// itself changes; goldenWidths never lands on it.
	widths := append([]int{99, 100}, goldenWidths...)
	for _, w := range widths {
		for _, lang := range goldenLanguages {
			for name, model := range models {
				t.Run(fmt.Sprintf("%s_%s_%d", name, lang.name, w), func(t *testing.T) {
					i18n.SetLanguage(lang.tag)
					t.Cleanup(func() { i18n.SetLanguage(language.English) })
					for i, line := range strings.Split(model(w).View(), "\n") {
						if got := ansi.StringWidth(line); got > w {
							t.Errorf("line %d is %d columns wide in a terminal %d wide: %q",
								i, got, w, ansi.Strip(line))
						}
					}
				})
			}
		}
	}
}

func TestNoUnresolvedIDsInTheDiffView(t *testing.T) {
	for _, lang := range goldenLanguages {
		t.Run(lang.name, func(t *testing.T) {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			loading := New(&fakeSource{}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 1})
			loading, _ = loading.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			threads := withThreads(t, 120, 40)
			expanded := press(openCollapsedThread(threads), "enter")
			for name, view := range map[string]string{
				"loaded":         loaded(t, 120, 30).View(),
				"loading":        loading.View(),
				"empty":          emptyDiff(t, 120, 30).View(),
				"threads":        threads.View(),
				"expanded":       expanded.View(),
				"pending":        withPending(t).View(),
				"review_failure": reviewFailureModel(120).View(),
				"composing":      composingModel(120).View(),
				"posting":        postingModel(120).View(),
				"submitting":     submittingModel(120).View(),
				"discarding":     discardingModel(120).View(),
			} {
				t.Run(name, func(t *testing.T) {
					i18n.AssertNoUnresolvedIDs(t, view)
				})
			}
		})
	}
}
