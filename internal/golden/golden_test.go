package golden_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/golden"
)

// styled carries the escapes a recording exists to protect: a change that
// drops the colour has to show up as a diff.
const styled = "\x1b[1mbold\x1b[m \x1b[38;2;255;0;0mred\x1b[m\n"

// inTempDir moves the test into a scratch directory, because Assert reads and
// writes testdata relative to the working directory.
func inTempDir(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})
}

func TestUpdateWritesTheEscapesUnchanged(t *testing.T) {
	inTempDir(t)
	t.Setenv(golden.UpdateEnv, "1")

	golden.Assert(t, "sample", styled)

	written, err := os.ReadFile(filepath.Join("testdata", "sample.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != styled {
		t.Errorf("the recording is not byte-for-byte what was rendered:\n%q", written)
	}
	if !strings.Contains(string(written), "\x1b[") {
		t.Error("the recording has lost its ANSI escapes")
	}
}

func TestAMatchingRecordingPasses(t *testing.T) {
	inTempDir(t)
	t.Setenv(golden.UpdateEnv, "1")
	golden.Assert(t, "sample", styled)

	t.Setenv(golden.UpdateEnv, "")
	golden.Assert(t, "sample", styled) // fails the test if it does not match
}

// TestADifferenceInStyleAloneIsCaught is the assertion the harness exists for:
// the text is identical and only the escapes differ.
func TestADifferenceInStyleAloneIsCaught(t *testing.T) {
	inTempDir(t)
	t.Setenv(golden.UpdateEnv, "1")
	golden.Assert(t, "sample", styled)

	t.Setenv(golden.UpdateEnv, "")
	stripped := "bold red\n"
	spy := &testing.T{}
	golden.Assert(spy, "sample", stripped)
	if !spy.Failed() {
		t.Error("losing every escape did not fail the comparison")
	}
}

func TestAMissingRecordingSaysHowToMakeOne(t *testing.T) {
	inTempDir(t)
	t.Setenv(golden.UpdateEnv, "")

	spy := &testing.T{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		golden.Assert(spy, "never-recorded", styled) // Fatalf ends this goroutine
	}()
	<-done
	if !spy.Failed() {
		t.Error("a missing recording passed")
	}
}
