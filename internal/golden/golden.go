// Package golden records what a view draws, so that a change to the drawing
// shows up as a diff a human can read.
//
// The recordings keep their ANSI escapes. A change that adds or drops colour
// is exactly the kind of change these files exist to catch, and stripping the
// escapes would hide it.
package golden

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// dir is the only directory a recording is ever read from or written to.
const dir = "testdata"

// UpdateEnv is the environment variable that rewrites the recordings instead
// of comparing against them: OCTOSCOPE_UPDATE_GOLDEN=1 go test ./...
//
// It is an environment variable rather than a test flag because `go test ./...`
// hands every flag to every package, and the packages without golden tests
// would reject an unknown one.
const UpdateEnv = "OCTOSCOPE_UPDATE_GOLDEN"

// Assert compares got against testdata/<name>.golden.
func Assert(t *testing.T, name, got string) {
	t.Helper()

	file := name + ".golden"
	path := filepath.Join(dir, file)
	if os.Getenv(UpdateEnv) != "" {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Reading through a filesystem rooted at testdata, rather than joining the
	// name onto a path, is what keeps the helper from being able to reach
	// anywhere else.
	want, err := fs.ReadFile(os.DirFS(dir), file)
	if err != nil {
		t.Fatalf("%v\nrun with %s=1 to record it", err, UpdateEnv)
	}
	if got != string(want) {
		t.Errorf("%s does not match.\n--- want ---\n%s\n--- got ---\n%s\n"+
			"run with %s=1 to accept the new output", path, want, got, UpdateEnv)
	}
}
