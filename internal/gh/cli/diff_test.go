package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func readSample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sample.diff")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sampleFiles(t *testing.T) []gh.FileDiff {
	t.Helper()
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) { return readSample(t), nil }
	files, err := c.PRDiff(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestPRDiffBuildsTheCommand(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return readSample(t), nil
	}
	if _, err := c.PRDiff(context.Background(), "", 128); err != nil {
		t.Fatal(err)
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

func TestPRDiffParsesEveryShape(t *testing.T) {
	files := sampleFiles(t)
	if len(files) != 7 {
		t.Fatalf("parsed %d files, want 7", len(files))
	}

	tests := []struct {
		name      string
		file      gh.FileDiff
		path      string
		oldPath   string
		status    gh.FileStatus
		additions int
		deletions int
		binary    bool
		hunks     int
	}{
		{"modified", files[0], "graph/walk.go", "", gh.FileModified, 4, 1, false, 2},
		{"added", files[1], "graph/new.go", "", gh.FileAdded, 2, 0, false, 1},
		{"deleted", files[2], "graph/old.go", "", gh.FileDeleted, 0, 1, false, 1},
		{"renamed", files[3], "docs/b.md", "docs/a.md", gh.FileRenamed, 1, 1, false, 1},
		{"binary", files[4], "logo.png", "", gh.FileModified, 0, 0, true, 0},
		{"no trailing newline", files[5], "noeol.txt", "", gh.FileModified, 1, 1, false, 1},
		{"hunk header with function context", files[6], "mathutil/add.go", "", gh.FileModified, 3, 1, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.file
			if f.Path != tt.path || f.OldPath != tt.oldPath {
				t.Errorf("path = %q (old %q), want %q (old %q)", f.Path, f.OldPath, tt.path, tt.oldPath)
			}
			if f.Status != tt.status {
				t.Errorf("status = %v, want %v", f.Status, tt.status)
			}
			if f.Additions != tt.additions || f.Deletions != tt.deletions {
				t.Errorf("+%d -%d, want +%d -%d", f.Additions, f.Deletions, tt.additions, tt.deletions)
			}
			if f.Binary != tt.binary {
				t.Errorf("binary = %v, want %v", f.Binary, tt.binary)
			}
			if len(f.Hunks) != tt.hunks {
				t.Errorf("%d hunks, want %d", len(f.Hunks), tt.hunks)
			}
		})
	}
}

// TestLineNumbersRunDownBothSides is the test the whole parser exists for: a
// comment posts to a line number on a side, so a wrong number here puts the
// comment on the wrong line of a real pull request.
func TestLineNumbersRunDownBothSides(t *testing.T) {
	hunk := sampleFiles(t)[0].Hunks[0]

	got := make([][3]int, 0, len(hunk.Lines))
	for _, l := range hunk.Lines {
		got = append(got, [3]int{int(l.Kind), l.OldLine, l.NewLine})
	}
	want := [][3]int{
		{int(gh.LineContext), 12, 12},
		{int(gh.LineContext), 13, 13},
		{int(gh.LineRemoved), 14, 0},
		{int(gh.LineAdded), 0, 14},
		{int(gh.LineAdded), 0, 15},
		{int(gh.LineAdded), 0, 16},
		{int(gh.LineContext), 15, 17},
		{int(gh.LineContext), 16, 18},
	}
	if len(got) != len(want) {
		t.Fatalf("%d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestHunkHeaderFunctionContextDoesNotShiftLineNumbers guards against a hunk
// header like `@@ -3,4 +3,5 @@ func add(a, b int) int { return a + b }`: the
// function-context text git appends after the second `@@` can contain a
// token starting with `+` or `-` (here, the literal `+` in the signature),
// which must not be mistaken for the start-of-hunk fields.
func TestHunkHeaderFunctionContextDoesNotShiftLineNumbers(t *testing.T) {
	hunk := sampleFiles(t)[6].Hunks[0]

	got := make([][3]int, 0, len(hunk.Lines))
	for _, l := range hunk.Lines {
		got = append(got, [3]int{int(l.Kind), l.OldLine, l.NewLine})
	}
	want := [][3]int{
		{int(gh.LineContext), 3, 3},
		{int(gh.LineContext), 4, 4},
		{int(gh.LineRemoved), 5, 0},
		{int(gh.LineAdded), 0, 5},
		{int(gh.LineAdded), 0, 6},
		{int(gh.LineAdded), 0, 7},
	}
	if len(got) != len(want) {
		t.Fatalf("%d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %v, want %v", i, got[i], want[i])
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
	files, err := c.PRDiff(context.Background(), "", 412)
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
	if len(files) != 1 || files[0].Path != "a.go" || files[0].Additions != 1 || files[0].Deletions != 1 {
		t.Errorf("files = %+v, want one parsed file", files)
	}
	if len(files[0].Hunks) != 1 || len(files[0].Hunks[0].Lines) != 2 {
		t.Fatalf("hunks = %+v, want one hunk of two lines", files[0].Hunks)
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

// TestBarePatchParserMatchesTheFullDiffParser guards the files API's patch
// shape: no "diff --git" header, no ---/+++ lines, just the hunks. It shares
// hunkStarts with the full-diff parser, so the function-context fix that
// landed there must not need a second copy here.
func TestBarePatchParserMatchesTheFullDiffParser(t *testing.T) {
	patch := "@@ -3,4 +3,5 @@ func add(a, b int) int { return a + b }\n" +
		" x := a\n" +
		"-y := b\n" +
		"+y := b + 1\n" +
		"+z := 0\n" +
		" return x + y"
	hunks := parseBarePatch(patch)
	if len(hunks) != 1 {
		t.Fatalf("%d hunks, want 1", len(hunks))
	}
	got := make([][3]int, 0, len(hunks[0].Lines))
	for _, l := range hunks[0].Lines {
		got = append(got, [3]int{int(l.Kind), l.OldLine, l.NewLine})
	}
	want := [][3]int{
		{int(gh.LineContext), 3, 3},
		{int(gh.LineRemoved), 4, 0},
		{int(gh.LineAdded), 0, 4},
		{int(gh.LineAdded), 0, 5},
		{int(gh.LineContext), 5, 6},
	}
	if len(got) != len(want) {
		t.Fatalf("%d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestFileStatusFromAPIMapsEveryValue guards the files API's status
// spelling, including GitHub saying "removed" rather than "deleted".
func TestFileStatusFromAPIMapsEveryValue(t *testing.T) {
	tests := []struct {
		api  string
		want gh.FileStatus
	}{
		{"added", gh.FileAdded},
		{"removed", gh.FileDeleted},
		{"modified", gh.FileModified},
		{"renamed", gh.FileRenamed},
		{"copied", gh.FileCopied},
		{"changed", gh.FileChanged},
		{"unchanged", gh.FileUnchanged},
	}
	for _, tt := range tests {
		t.Run(tt.api, func(t *testing.T) {
			if got := fileStatusFromAPI(tt.api); got != tt.want {
				t.Errorf("fileStatusFromAPI(%q) = %v, want %v", tt.api, got, tt.want)
			}
		})
	}
}

// TestPRFileWithNoPatchIsMarkedOmitted covers a file too large or binary for
// GitHub to send a patch for: absent, not empty, and different from Binary.
func TestPRFileWithNoPatchIsMarkedOmitted(t *testing.T) {
	e := prFileJSON{Filename: "huge.bin", Status: "modified"}
	f := e.toDomain()
	if !f.PatchOmitted {
		t.Error("PatchOmitted = false, want true for a file with no patch field")
	}
	if len(f.Hunks) != 0 {
		t.Errorf("hunks = %v, want none", f.Hunks)
	}
}

// TestPRFileRenameCarriesBothPaths guards previous_filename -> OldPath.
func TestPRFileRenameCarriesBothPaths(t *testing.T) {
	e := prFileJSON{Filename: "b.md", PreviousFilename: "a.md", Status: "renamed"}
	f := e.toDomain()
	if f.Path != "b.md" || f.OldPath != "a.md" {
		t.Errorf("path = %q, old = %q, want b.md / a.md", f.Path, f.OldPath)
	}
	if f.Status != gh.FileRenamed {
		t.Errorf("status = %v, want FileRenamed", f.Status)
	}
}

func TestDiffLineNamesTheSideToCommentOn(t *testing.T) {
	tests := []struct {
		name string
		line gh.DiffLine
		num  int
		side gh.DiffSide
	}{
		{"removed lines quote the left", gh.DiffLine{Kind: gh.LineRemoved, OldLine: 14}, 14, gh.SideLeft},
		{"added lines quote the right", gh.DiffLine{Kind: gh.LineAdded, NewLine: 15}, 15, gh.SideRight},
		{"context quotes the right", gh.DiffLine{Kind: gh.LineContext, OldLine: 12, NewLine: 12}, 12, gh.SideRight},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			num, side := tt.line.Line()
			if num != tt.num || side != tt.side {
				t.Errorf("Line() = %d %v, want %d %v", num, side, tt.num, tt.side)
			}
		})
	}
}
