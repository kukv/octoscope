package cli

import (
	"context"
	"os"
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
