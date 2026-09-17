package checks

import "testing"

// TestLogLinesClearedWithLog locks what the cached rows must follow: the
// log they were built from. A cache left behind after clearLog would draw
// the previous check's log under the check the cursor moved to.
func TestLogLinesClearedWithLog(t *testing.T) {
	m := goldenModel(160)
	m, _ = m.Update(keyPress("enter"))
	m = m.logArrived(logMsg{ref: m.ref, jobID: m.selectedJob(), lines: goldenLog()})

	if len(m.logRows()) == 0 {
		t.Fatal("no rows after a log arrived")
	}

	m = m.clearLog()

	if got := m.logRows(); got != nil {
		t.Fatalf("rows survived clearLog: %q", got)
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
