package checks

import (
	"fmt"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
)

// benchSink keeps the compiler from optimising View away.
var benchSink string

// hugeLog is a job log of the size a long Actions run actually produces.
// Every tenth line starts a new step, so the step headings are in the
// measurement too.
func hugeLog(n int) []domain.LogLine {
	at := time.Date(2026, 9, 7, 10, 15, 30, 0, time.UTC)
	lines := make([]domain.LogLine, 0, n)
	for i := range n {
		lines = append(lines, domain.LogLine{
			Step: fmt.Sprintf("Run step %d", i/10),
			Time: at.Add(time.Duration(i) * time.Second),
			Text: fmt.Sprintf("go: downloading github.com/example/module/v%d v1.2.3", i),
		})
	}
	return lines
}

// BenchmarkViewHugeLog is one frame with a 50,000-line log open. Only a
// screenful is drawn, so the cost must not follow the log's length.
func BenchmarkViewHugeLog(b *testing.B) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: hugeLog(50000)})
	for b.Loop() {
		benchSink = m.View()
	}
}

// BenchmarkMoveRowHugeLog is one keypress in the log pane: it clamps the
// cursor and re-bounds the horizontal offset, each of which asks for the
// log's rows.
func BenchmarkMoveRowHugeLog(b *testing.B) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: hugeLog(50000)})
	m.pane = paneLog
	for b.Loop() {
		m = m.moveRow(1)
	}
}
