package gh

import (
	"bytes"
	"os"
	"reflect"
	"testing"
)

func readSample(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/sample.diff")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sampleFiles(t *testing.T) []FileDiff {
	t.Helper()
	return ParseDiff(readSample(t))
}

func TestPRDiffParsesEveryShape(t *testing.T) {
	files := sampleFiles(t)
	if len(files) != 7 {
		t.Fatalf("parsed %d files, want 7", len(files))
	}

	tests := []struct {
		name      string
		file      FileDiff
		path      string
		oldPath   string
		status    FileStatus
		additions int
		deletions int
		binary    bool
		hunks     int
	}{
		{"modified", files[0], "graph/walk.go", "", FileModified, 4, 1, false, 2},
		{"added", files[1], "graph/new.go", "", FileAdded, 2, 0, false, 1},
		{"deleted", files[2], "graph/old.go", "", FileDeleted, 0, 1, false, 1},
		{"renamed", files[3], "docs/b.md", "docs/a.md", FileRenamed, 1, 1, false, 1},
		{"binary", files[4], "logo.png", "", FileModified, 0, 0, true, 0},
		{"no trailing newline", files[5], "noeol.txt", "", FileModified, 1, 1, false, 1},
		{"hunk header with function context", files[6], "mathutil/add.go", "", FileModified, 3, 1, false, 1},
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
		{int(LineContext), 12, 12},
		{int(LineContext), 13, 13},
		{int(LineRemoved), 14, 0},
		{int(LineAdded), 0, 14},
		{int(LineAdded), 0, 15},
		{int(LineAdded), 0, 16},
		{int(LineContext), 15, 17},
		{int(LineContext), 16, 18},
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
		{int(LineContext), 3, 3},
		{int(LineContext), 4, 4},
		{int(LineRemoved), 5, 0},
		{int(LineAdded), 0, 5},
		{int(LineAdded), 0, 6},
		{int(LineAdded), 0, 7},
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

// TestBarePatchParserMatchesTheFullDiffParser guards the files API's patch
// shape: no "diff --git" header, no ---/+++ lines, just the hunks. It feeds
// the mathutil/add.go fixture from sample.diff — whose hunk header carries a
// "+" in its function context, the exact shape that once shifted line
// numbers — through both entry points and requires the same Hunks out,
// rather than hardcoding expected values a copy of hunkStarts could also
// satisfy: this is what actually pins the two paths sharing hunkStarts.
func TestBarePatchParserMatchesTheFullDiffParser(t *testing.T) {
	sample := readSample(t)
	const marker = "diff --git a/mathutil/add.go b/mathutil/add.go"
	i := bytes.Index(sample, []byte(marker))
	if i < 0 {
		t.Fatal("testdata/sample.diff no longer has the mathutil/add.go fixture")
	}
	full := ParseDiff(sample[i:])
	if len(full) != 1 {
		t.Fatalf("%d files from the full-diff parser, want 1", len(full))
	}

	j := bytes.IndexByte(sample[i:], '@')
	if j < 0 {
		t.Fatal("no hunk header found after the diff --git line")
	}
	bare := parseBarePatch(string(sample[i+j:]))

	if !reflect.DeepEqual(bare, full[0].Hunks) {
		t.Errorf("bare-patch hunks = %+v, want the same as the full-diff parser: %+v", bare, full[0].Hunks)
	}
}

// TestFileStatusFromAPIMapsEveryValue guards the files API's status
// spelling, including GitHub saying "removed" rather than "deleted".
func TestFileStatusFromAPIMapsEveryValue(t *testing.T) {
	tests := []struct {
		api  string
		want FileStatus
	}{
		{"added", FileAdded},
		{"removed", FileDeleted},
		{"modified", FileModified},
		{"renamed", FileRenamed},
		{"copied", FileCopied},
		{"changed", FileChanged},
		{"unchanged", FileUnchanged},
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
	if f.Status != FileRenamed {
		t.Errorf("status = %v, want FileRenamed", f.Status)
	}
}
