package github_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/github"
)

// TestPRFileWithNoPatchIsMarkedOmitted covers a file too large or binary for
// GitHub to send a patch for: the JSON has no patch field at all, which must
// decode to a nil Patch, not an empty string.
func TestPRFileWithNoPatchIsMarkedOmitted(t *testing.T) {
	files, err := github.ParseFilesAPI([]byte(`[{"filename":"huge.bin","status":"modified"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if files[0].Patch != nil {
		t.Errorf("Patch = %v, want nil for a file with no patch field", *files[0].Patch)
	}
}

// TestPRFileRenameCarriesBothPaths guards previous_filename -> PreviousFilename.
func TestPRFileRenameCarriesBothPaths(t *testing.T) {
	files, err := github.ParseFilesAPI([]byte(
		`[{"filename":"b.md","previous_filename":"a.md","status":"renamed"}]`))
	if err != nil {
		t.Fatal(err)
	}
	f := files[0]
	if f.Filename != "b.md" || f.PreviousFilename != "a.md" {
		t.Errorf("filename = %q, previous = %q, want b.md / a.md", f.Filename, f.PreviousFilename)
	}
	if f.Status != "renamed" {
		t.Errorf("status = %q, want renamed", f.Status)
	}
}
