package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// readSample reads the one recording of sample.diff, kept in internal/app/domain
// since that is where the parsing tests that assert on its content now live.
// The tests here only need gh pr diff to return something a real diff could
// be, not any particular content of it.
func readSample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../../app/domain/testdata/sample.diff")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPRDiffBuildsTheCommand(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	sample := readSample(t)
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return sample, nil
	}
	diff, err := c.PRDiff(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(diff.Raw, sample) || diff.Files != nil {
		t.Fatal("PRDiff() did not return gh pr diff's raw output as Raw")
	}
	want := []string{"pr", "diff", "128", "--color", "never", "--repo", "kukv/koto"}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}
}

// TestPRDiffFallsBackToTheFilesAPI covers the case the user hit: gh pr diff
// refuses past 300 files (HTTP 406), and the files API has no such limit.
func TestPRDiffFallsBackToTheFilesAPI(t *testing.T) {
	c := New("/w", "kukv/koto")
	var calls [][]string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls = append(calls, args)
		if len(calls) == 1 {
			return nil, errors.New("gh pr diff: HTTP 406: too many files")
		}
		return []byte(`[{"filename":"a.go","status":"modified","additions":1,"deletions":1,"patch":"@@ -1,1 +1,1 @@\n-old\n+new"}]`), nil
	}
	diff, err := c.PRDiff(context.Background(), "", 412)
	if err != nil {
		t.Fatalf("PRDiff() error = %v, want nil", err)
	}
	if len(calls) != 2 {
		t.Fatalf("gh was run %d times, want 2 (pr diff, then the fallback)", len(calls))
	}
	fallback := calls[1]
	if fallback[0] != "api" {
		t.Fatalf("fallback args = %v, want an `api` call", fallback)
	}
	found := false
	for _, a := range fallback {
		if a == "--paginate" {
			found = true
		}
	}
	if !found {
		t.Errorf("fallback args = %v, want --paginate (this pull request has more files than one page)", fallback)
	}
	if !strings.Contains(fallback[1], "per_page=100") {
		t.Errorf("fallback path = %q, want per_page=100 (30 files a page is 14 requests for this pull request)", fallback[1])
	}
	if diff.Raw != nil {
		t.Errorf("Raw = %v, want nil once the fallback answered", diff.Raw)
	}
	if len(diff.Files) != 1 || diff.Files[0].Filename != "a.go" || diff.Files[0].Additions != 1 || diff.Files[0].Deletions != 1 {
		t.Errorf("files = %+v, want one parsed file", diff.Files)
	}
}

// TestPRDiffReportsTheOriginalErrorWhenBothFail guards against masking gh pr
// diff's own failure (the one that describes what the user actually asked
// for) with whatever the fallback says instead.
func TestPRDiffReportsTheOriginalErrorWhenBothFail(t *testing.T) {
	c := New("/w", "kukv/koto")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("gh pr diff: HTTP 406: too many files")
		}
		return nil, errors.New("gh api: HTTP 404: not found")
	}
	_, err := c.PRDiff(context.Background(), "", 412)
	if err == nil || !strings.Contains(err.Error(), "406") {
		t.Errorf("PRDiff() error = %v, want the gh pr diff error", err)
	}
}

// TestPRDiffKeepsErrGhNotFoundWhenBothCallsFail guards the sentinel root.fail
// checks with errors.Is: joining the fallback's error in must not stop
// ErrGhNotFound from still being found in the result.
func TestPRDiffKeepsErrGhNotFoundWhenBothCallsFail(t *testing.T) {
	c := New("/w", "kukv/koto")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, domain.ErrGhNotFound
		}
		return nil, errors.New("gh api: HTTP 404: not found")
	}
	_, err := c.PRDiff(context.Background(), "", 412)
	if !errors.Is(err, domain.ErrGhNotFound) {
		t.Errorf("PRDiff() error = %v, want errors.Is(err, domain.ErrGhNotFound)", err)
	}
}

// prFiles is what PRDiff falls back to when `gh pr diff` fails; the recording
// proves ParseFilesAPI is fed the shape GitHub actually sends there.
func TestPRFilesParsesARecordedResponse(t *testing.T) {
	c, _ := newTestClient(readTestdata(t, "pr_files.json"), nil)

	files, err := c.prFiles(t.Context(), "", 55)
	if err != nil {
		t.Fatalf("prFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed out of the recording")
	}
	for _, f := range files {
		if f.Filename == "" {
			t.Error("a file has no filename")
		}
	}
}
