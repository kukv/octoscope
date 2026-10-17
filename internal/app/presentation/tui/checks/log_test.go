package checks

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/i18n"
)

// TestLogLinesClearedWithLog locks that clearLog drops logLines together
// with log: a clearLog that dropped only one of the two would leave the
// previous check's log to be drawn under the check the cursor moved to.
func TestLogLinesClearedWithLog(t *testing.T) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: goldenLog()})

	if len(m.logLines) == 0 {
		t.Fatal("no rows after a log arrived")
	}

	m = m.clearLog()

	if m.logLines != nil {
		t.Fatalf("logLines survived clearLog: %q", m.logLines)
	}
}

// TestLogRowsBuiltOnce is why the rows are kept at all: View, the cursor
// clamp and the horizontal bound each ask for them, and a job log runs to
// tens of thousands of lines. Asking twice must not build twice.
func TestLogRowsBuiltOnce(t *testing.T) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: goldenLog()})

	first := m.logRows()
	second := m.logRows()

	if len(first) == 0 {
		t.Fatal("no rows after a log arrived")
	}
	if &first[0] != &second[0] {
		t.Fatal("logRows built the rows a second time")
	}
}

// GitHub no longer splits a run's log archive by step, so a log can come
// back as the job's whole log with no step named on any line, failed steps
// asked for or not. Headed "failed steps", it reads as the failing steps
// alone when it is every step's output.
func TestALogWithNoStepsIsNotCalledFailedSteps(t *testing.T) {
	whole := []domain.LogLine{
		{Time: time.Date(2026, 9, 7, 10, 15, 30, 0, time.UTC), Text: "Run actions/checkout@v4"},
		{Text: "FAIL ./internal/gh"},
	}
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: whole})

	out := ansi.Strip(m.View())
	if !strings.Contains(out, i18n.T("checks.log_whole_job")) {
		t.Errorf("the heading does not say it is the whole job's log:\n%s", out)
	}
	if strings.Contains(out, i18n.T("checks.log_failed_only")) {
		t.Errorf("the heading still says failed steps:\n%s", out)
	}
	if n := len(m.logLines); n != len(whole) {
		t.Errorf("%d rows for %d lines, want no step heading row above them: %q", n, len(whole), m.logLines)
	}
}

// A log with steps named is what failed steps means, and keeps that heading.
func TestALogWithStepsIsStillCalledFailedSteps(t *testing.T) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: goldenLog()})

	out := ansi.Strip(m.View())
	if !strings.Contains(out, i18n.T("checks.log_failed_only")) {
		t.Errorf("the heading does not say failed steps:\n%s", out)
	}
}
