package diff

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
)

// benchSink keeps the compiler from optimising View away.
var benchSink string

// BenchmarkView is one frame of the diff pane at the width a screen most
// commonly opens at. Bubble Tea calls View once per message, so this is
// what a single keypress costs.
func BenchmarkView(b *testing.B) {
	m := goldenModel(160)
	for b.Loop() {
		benchSink = m.View()
	}
}

// hugeDiff is a pull request of the size that made the diff view feel slow:
// many files, and a first file long enough that its line numbers run past
// four digits, which is what widens the gutter.
func hugeDiff(files, linesPerFile int) []domain.FileDiff {
	out := make([]domain.FileDiff, 0, files)
	for f := range files {
		lines := make([]domain.DiffLine, 0, linesPerFile)
		for i := range linesPerFile {
			lines = append(lines, domain.DiffLine{
				Kind:    domain.LineContext,
				OldLine: i + 1,
				NewLine: i + 1,
				Text:    "\tif err := walk(ctx, node, depth+1); err != nil {",
			})
		}
		out = append(out, domain.FileDiff{
			Path:      fmt.Sprintf("internal/app/pkg%d/file%d.go", f/10, f),
			Additions: linesPerFile / 2,
			Deletions: linesPerFile / 4,
			Hunks:     []domain.Hunk{{Header: "@@ -1,1 +1,1 @@", Lines: lines}},
		})
	}
	return out
}

// hugeModel is the diff view opened on hugeDiff, with a review context
// carrying a thread per file: threadCount walks every thread for every file
// the sidebar draws, so the threads have to be there to measure it.
//
// The width is the one a screen most commonly opens at, and the file count
// is what a large pull request runs to; a caller that needs another size
// sends its own WindowSizeMsg. To check that the cost no longer follows the
// size of the pull request, raise files here and see that the benchmark
// barely moves.
func hugeModel(linesPerFile int) Model {
	const (
		width = 160
		files = 300
	)

	diff := hugeDiff(files, linesPerFile)
	threads := make([]domain.ReviewThread, 0, len(diff))
	for _, f := range diff {
		threads = append(threads, domain.ReviewThread{
			Path: f.Path, Line: 1, Side: domain.SideRight,
			Comments: []domain.ThreadComment{
				{Author: domain.Author{Login: "kukv"}, Body: "ここは 2 が既定ではないでしょうか"},
			},
		})
	}
	m := New(&fakeSource{files: diff}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(diffMsg{ref: m.ref, files: diff})
	m, _ = m.Update(reviewMsg{ref: m.ref, ctx: domain.ReviewContext{Threads: threads}})
	return m
}

// BenchmarkViewHugeDiff is one frame of a 300-file pull request whose open
// file runs to 5,000 lines. Only a screenful is drawn, so neither number
// should reach the cost.
func BenchmarkViewHugeDiff(b *testing.B) {
	m := hugeModel(5000)
	if len(m.rows) < 5000 {
		b.Fatalf("the diff did not land: %d rows", len(m.rows))
	}
	for b.Loop() {
		benchSink = m.View()
	}
}
