package api

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// Rerunning everything and rerunning only what failed are two endpoints, not
// one endpoint with a flag. Sending the whole run to the failed-jobs path
// would start jobs that already passed.
func TestRerunScopePicksTheEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		scope domain.RerunScope
		want  string
	}{
		{"all", domain.RerunAll, "/repos/kukv/octoscope/actions/runs/61/rerun"},
		{"failed", domain.RerunFailed, "/repos/kukv/octoscope/actions/runs/61/rerun-failed-jobs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{}`)
			})
			if err := c.RerunWorkflow(context.Background(), "kukv/octoscope", 61, tc.scope); err != nil {
				t.Fatalf("RerunWorkflow: %v", err)
			}

			req := (*got)[0]
			if req.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", req.Method)
			}
			if req.URL.Path != tc.want {
				t.Errorf("path = %q, want %q", req.URL.Path, tc.want)
			}
		})
	}
}

// A rerun is not repeated when GitHub's front end fails to answer: a 502 says
// no answer came back, not that nothing happened, and a second POST would
// start the run twice.
func TestARerunIsNotRepeatedOnATransientFailure(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"message": "Server Error"}`)
	})
	if err := c.RerunWorkflow(context.Background(), "kukv/octoscope", 61, domain.RerunAll); err == nil {
		t.Fatal("RerunWorkflow: want an error")
	}
	if len(*got) != 1 {
		t.Errorf("requests = %d, want 1: a rerun must not be sent twice", len(*got))
	}
}

// gh refuses a job that has not finished, and the TUI shows GitHub's sentence
// as it is. A different wording here would put a different sentence on the
// screen depending on which backend is running.
func TestAnInProgressJobIsRefusedInGhsOwnWords(t *testing.T) {
	t.Parallel()

	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"id": 61, "run_id": 7, "name": "build", "status": "in_progress"}`)
	})

	_, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err == nil {
		t.Fatal("JobLog: want an error for a job that has not finished")
	}
	want := "job 61 is still in progress; logs will be available when it is complete"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

// A job that passed has no failed steps to show. gh prints nothing and exits
// zero, and the cli backend already treats an empty log as an answer; asking
// GitHub for the run's whole log archive here would be a download nobody reads.
func TestFailedOnlyOnAPassingJobAnswersWithNothing(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w,
			`{"id": 61, "run_id": 7, "name": "build", "status": "completed", "conclusion": "success"}`)
	})

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, true)
	if err != nil {
		t.Fatalf("JobLog: %v, want no error: an empty log is not a failure", err)
	}
	if len(lines) != 0 {
		t.Errorf("lines = %d, want none", len(lines))
	}
	if len(*got) != 1 {
		t.Errorf("requests = %d, want 1: the log archive must not be fetched", len(*got))
	}
}

// A job GitHub marks action_required or timed_out failed too. Treating only
// "failure" as a failure would show an empty log for a job whose red mark is
// exactly what the user clicked.
func TestTheOtherFailedConclusionsCountAsFailed(t *testing.T) {
	t.Parallel()

	for _, conclusion := range []string{"failure", "startup_failure", "timed_out", "action_required"} {
		t.Run(conclusion, func(t *testing.T) {
			t.Parallel()

			var calls int
			c, _ := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if strings.HasSuffix(r.URL.Path, "/logs") {
					w.WriteHeader(http.StatusNotFound)
					_, _ = io.WriteString(w, `{"message": "Not Found"}`)
					return
				}
				_, _ = io.WriteString(w, `{"id": 61, "run_id": 7, "name": "build", "status": "completed",
					"conclusion": "`+conclusion+`"}`)
			})

			// The log fetch is expected to fail here: what this test checks is
			// that JobLog went looking for it at all.
			_, _ = c.JobLog(context.Background(), "kukv/octoscope", 61, true)
			if calls < 2 {
				t.Errorf("requests = %d, want the log to be fetched for a %s job", calls, conclusion)
			}
		})
	}
}

// zipOf builds a log archive the way GitHub serves one, so the test's
// expectations are readable: a binary fixture would hide what is in it.
func zipOf(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := io.WriteString(f, body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// serveJobLog answers the two requests JobLog makes: the job, then the run's
// log archive.
func serveJobLog(t *testing.T, j string, archive []byte) (*Client, *[]*http.Request) {
	t.Helper()

	return serveREST(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/logs") {
			_, _ = w.Write(archive)
			return
		}
		_, _ = io.WriteString(w, j)
	})
}

const twoStepJob = `{"id": 61, "run_id": 7, "name": "build", "status": "completed",
	"conclusion": "failure", "steps": [
		{"name": "Set up job", "number": 1, "conclusion": "success"},
		{"name": "Run tests", "number": 2, "conclusion": "failure"}]}`

// A step's name is not in the log's text: it is in the name of the file the
// step's output was written to. Reading the log without the entry names would
// leave every line unattributed.
func TestEachLineCarriesTheStepItCameFrom(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "2026-09-07T09:15:20.0000000Z starting\n",
		"build/2_Run tests.txt":  "2026-09-07T09:15:22.0000000Z FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if lines[0].Step != "Set up job" || lines[1].Step != "Run tests" {
		t.Errorf("steps = %q, %q", lines[0].Step, lines[1].Step)
	}
	if lines[0].Text != "starting" || lines[1].Text != "FAIL" {
		t.Errorf("text = %q, %q", lines[0].Text, lines[1].Text)
	}
}

// The steps come out in the order they ran. A zip's entries are in whatever
// order the archive was written in, and a log read out of order is unreadable.
func TestStepsComeOutInTheOrderTheyRan(t *testing.T) {
	t.Parallel()

	// Ten steps, so that a sort by name would put "10_" before "2_".
	steps := make([]string, 0, 10)
	entries := map[string]string{}
	for i := 1; i <= 10; i++ {
		steps = append(steps, fmt.Sprintf(`{"name": "step %d", "number": %d, "conclusion": "success"}`, i, i))
		entries[fmt.Sprintf("build/%d_step %d.txt", i, i)] = fmt.Sprintf("line %d\n", i)
	}
	j := fmt.Sprintf(`{"id": 61, "run_id": 7, "name": "build", "status": "completed",
		"conclusion": "failure", "steps": [%s]}`, strings.Join(steps, ","))

	c, _ := serveJobLog(t, j, zipOf(t, entries))
	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 10 {
		t.Fatalf("lines = %d, want one per step", len(lines))
	}
	for i, line := range lines {
		if want := fmt.Sprintf("line %d", i+1); line.Text != want {
			t.Fatalf("lines[%d] = %q, want %q", i, line.Text, want)
		}
	}
}

// failedOnly narrows to the steps that failed, not to the jobs that failed:
// a failed job's passing steps are the part nobody opened the log to read.
func TestFailedOnlyKeepsOnlyTheStepsThatFailed(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "2026-09-07T09:15:20.0000000Z starting\n",
		"build/2_Run tests.txt":  "2026-09-07T09:15:22.0000000Z FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, true)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Step != "Run tests" {
		t.Fatalf("lines = %+v, want only the failed step", lines)
	}
}

// Some runs have no per-step entries at all, only one file for the whole job.
// Without this fallback those jobs would show an empty log.
func TestAJobWithNoStepEntriesFallsBackToItsWholeLog(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"0_build.txt": "2026-09-07T09:15:20.0000000Z starting\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "starting" {
		t.Fatalf("lines = %+v", lines)
	}
}

// The older Actions service names the whole-job file after the job's id, which
// is negative. A pattern that only allowed digits would miss it and show an
// empty log.
func TestTheLegacyWholeJobEntryIsFoundToo(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"-2147483648_build.txt": "2026-09-07T09:15:20.0000000Z starting\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "starting" {
		t.Fatalf("lines = %+v", lines)
	}
}

// An archive that was read but names neither this job's steps nor its whole
// log still leaves the job's own log endpoint to ask. Answering with nothing
// would look like a job that printed nothing.
func TestAnArchiveWithoutTheJobFallsBackToTheJobsOwnLog(t *testing.T) {
	t.Parallel()

	archive := zipOf(t, map[string]string{"0_other job.txt": "not this one\n"})
	c, got := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/actions/runs/7/logs"):
			_, _ = w.Write(archive)
		case strings.HasSuffix(r.URL.Path, "/actions/jobs/61/logs"):
			_, _ = io.WriteString(w, "2026-09-07T09:15:20.0000000Z starting\n")
		default:
			_, _ = io.WriteString(w, twoStepJob)
		}
	})

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "starting" {
		t.Fatalf("lines = %+v", lines)
	}
	if len(*got) != 3 {
		t.Errorf("requests = %d, want 3", len(*got))
	}
}

// A job whose name has a slash is stored under a directory without it. Looking
// for the job's own name would find nothing and fall all the way through to
// the single-job endpoint, losing every step name on the way.
func TestAJobNamedAfterACompositeActionFindsItsEntries(t *testing.T) {
	t.Parallel()

	j := `{"id": 61, "run_id": 7, "name": "build / test", "status": "completed",
		"conclusion": "failure", "steps": [{"name": "Run tests", "number": 1, "conclusion": "failure"}]}`
	c, _ := serveJobLog(t, j, zipOf(t, map[string]string{
		"build  test/1_Run tests.txt": "2026-09-07T09:15:22.0000000Z FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Step != "Run tests" {
		t.Fatalf("lines = %+v", lines)
	}
}

// An archive that cannot be fetched, or cannot be read as a zip, is an error
// and stops there. Falling back to the job's own log endpoint would turn a
// broken archive into a screenful of UNKNOWN STEP lines, where the cli backend
// stops with gh's error -- so what this checks is that the second endpoint was
// never asked.
func TestAnUnreadableArchiveIsAnErrorRatherThanAFallback(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		serve func(w http.ResponseWriter)
	}{
		{"archive is gone", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message": "Not Found"}`)
		}},
		{"archive is not a zip", func(w http.ResponseWriter) {
			_, _ = io.WriteString(w, "this is not an archive")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, got := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/actions/runs/7/logs") {
					tc.serve(w)
					return
				}
				_, _ = io.WriteString(w, twoStepJob)
			})

			if _, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false); err == nil {
				t.Fatal("JobLog: want an error for an archive that could not be read")
			}
			for _, req := range *got {
				if strings.HasSuffix(req.URL.Path, "/actions/jobs/61/logs") {
					t.Errorf("the job's own log was fetched: a broken archive must not fall back")
				}
			}
		})
	}
}

// A blank line inside a step's output is part of the output. gh prints an
// empty third field for it, which the cli backend keeps as a line with no
// text, so dropping it here would put a shorter log on the screen depending on
// which backend is running. The entry's own trailing newline is not a line.
func TestABlankLineInTheLogIsKeptTheWayGhKeepsIt(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "starting\n\ndone\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	var texts []string
	for _, line := range lines {
		texts = append(texts, line.Text)
	}
	if len(texts) != 3 || texts[0] != "starting" || texts[1] != "" || texts[2] != "done" {
		t.Fatalf("texts = %q, want the blank line kept and no line after the last newline", texts)
	}
}

// A step that printed nothing has an empty entry, and an empty entry is no
// lines. Splitting it the way a non-empty one is split would put one blank
// line on the screen for every silent step.
func TestAStepThatPrintedNothingContributesNoLines(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "",
		"build/2_Run tests.txt":  "FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Step != "Run tests" {
		t.Fatalf("lines = %+v, want only the step that printed something", lines)
	}
}

// GitHub lists a job's steps in the order it happens to hold them, and a log
// read out of order is unreadable. Leaving the order to the answer would make
// this depend on something nobody here controls.
func TestStepsOutOfOrderInTheAnswerStillComeOutInOrder(t *testing.T) {
	t.Parallel()

	j := `{"id": 61, "run_id": 7, "name": "build", "status": "completed",
		"conclusion": "failure", "steps": [
			{"name": "Run tests", "number": 2, "conclusion": "failure"},
			{"name": "Set up job", "number": 1, "conclusion": "success"}]}`
	c, _ := serveJobLog(t, j, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "starting\n",
		"build/2_Run tests.txt":  "FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 2 || lines[0].Step != "Set up job" || lines[1].Step != "Run tests" {
		t.Fatalf("lines = %+v, want the steps in the order they ran", lines)
	}
}

// The job JSON above is this test file's own; this one is GitHub's. A recorded
// job is what says run_id, steps[].number and the conclusions are spelled the
// way the decoding expects: get any of them wrong and the archive is fetched
// for run 0, or every step name is lost, or a skipped step reads as a failure.
func TestARecordedJobIsDecodedTheWayGitHubSpellsIt(t *testing.T) {
	t.Parallel()

	recorded, err := os.ReadFile("testdata/job.json")
	if err != nil {
		t.Fatalf("read the recorded job: %v", err)
	}
	// One entry per step the recorded job reports. Its numbers skip from 4 to
	// 8, which is what the real run's archive looks like.
	entries := map[string]string{}
	for _, s := range []struct {
		number int
		name   string
	}{
		{1, "Set up job"},
		{2, "Run actions-checkout"},
		{3, "scan source files"},
		{4, "scan AI rules files"},
		{8, "Post Run actions-checkout"},
		{9, "Complete job"},
	} {
		entries[fmt.Sprintf("hidden-unicode/%d_%s.txt", s.number, s.name)] = s.name + " ran\n"
	}
	archive := zipOf(t, entries)

	c, got := serveJobLog(t, string(recorded), archive)
	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 103558786732, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if want := "/repos/kukv/octoscope/actions/runs/34695737662/logs"; (*got)[1].URL.Path != want {
		t.Errorf("archive path = %q, want %q", (*got)[1].URL.Path, want)
	}
	if len(lines) != 6 {
		t.Fatalf("lines = %d, want one per step", len(lines))
	}
	if lines[0].Step != "Set up job" || lines[5].Step != "Complete job" {
		t.Errorf("steps = %q .. %q", lines[0].Step, lines[5].Step)
	}

	c, _ = serveJobLog(t, string(recorded), archive)
	failedLines, err := c.JobLog(context.Background(), "kukv/octoscope", 103558786732, true)
	if err != nil {
		t.Fatalf("JobLog with failedOnly: %v", err)
	}
	if len(failedLines) != 1 || failedLines[0].Step != "scan source files" {
		t.Fatalf("failed lines = %+v, want the one step the recorded job failed on", failedLines)
	}
}

// A step's output can carry an escape sequence -- a test runner's own colour
// codes, say. Handing it to the screen raw would let the log repaint the
// user's terminal, where the cli backend already shows it in caret notation.
func TestAnEscapeInAStepsOutputCannotReachTheScreen(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "starting\n",
		"build/2_Run tests.txt":  "\x1b[31mFAIL\x1b[0m\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 2 || lines[1].Text != "^[[31mFAIL^[[0m" {
		t.Fatalf("lines = %+v, want the escape shown as caret notation", lines)
	}
}

// A step name from GitHub's job JSON can carry a control character just as its
// log text can. gh's cli backend gets its step name through gh's own
// sanitizer, so leaving this one raw would show it differently depending on
// which backend answered.
func TestAnEscapeInAStepNameCannotReachTheScreen(t *testing.T) {
	t.Parallel()

	j := `{"id": 61, "run_id": 7, "name": "build", "status": "completed",
		"conclusion": "failure", "steps": [{"name": "\u001b[31mRun tests", "number": 1, "conclusion": "failure"}]}`
	c, _ := serveJobLog(t, j, zipOf(t, map[string]string{
		"build/1_\x1b[31mRun tests.txt": "FAIL\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Step != "^[[31mRun tests" {
		t.Fatalf("lines = %+v, want the step name shown as caret notation", lines)
	}
}

// The archive's line endings are CRLF, the way a Windows runner's job would
// write them. gh's own scanner drops the trailing \r, so an api backend that
// kept it would show a line the cli backend does not.
func TestACarriageReturnDoesNotSurviveTheLine(t *testing.T) {
	t.Parallel()

	c, _ := serveJobLog(t, twoStepJob, zipOf(t, map[string]string{
		"build/1_Set up job.txt": "starting\r\n",
		"build/2_Run tests.txt":  "FAIL\r\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	for _, line := range lines {
		if strings.Contains(line.Text, "\r") {
			t.Errorf("line %q carries a carriage return", line.Text)
		}
	}
}

// A reusable workflow's job is named "caller / callee", and the archive names
// its whole-job file after logFileName, with the slash stripped. Matching the
// raw job name here would miss the entry and send a third request to the
// job's own log endpoint for the same output.
func TestAWholeJobEntryForACompositeNameIsFoundWithoutAThirdRequest(t *testing.T) {
	t.Parallel()

	j := `{"id": 61, "run_id": 7, "name": "build / test", "status": "completed", "conclusion": "failure"}`
	c, got := serveJobLog(t, j, zipOf(t, map[string]string{
		"0_build  test.txt": "starting\n",
	}))

	lines, err := c.JobLog(context.Background(), "kukv/octoscope", 61, false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if len(lines) != 1 || lines[0].Text != "starting" {
		t.Fatalf("lines = %+v", lines)
	}
	if len(*got) != 2 {
		t.Errorf("requests = %d, want 2: the whole-job entry must be found without asking the job's own log", len(*got))
	}
}
