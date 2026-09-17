package checks

import "testing"

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
