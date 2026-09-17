package gql

import (
	"context"
	"strings"
	"testing"
)

// TestViewerReadsTheLogin is the whole of what this document is for: the one
// fact octoscope has never known about itself.
func TestViewerReadsTheLogin(t *testing.T) {
	t.Parallel()

	f := &fake{body: []byte(`{"data":{"viewer":{"login":"kukv"}}}`)}
	got, err := f.client().Viewer(context.Background())
	if err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if got != "kukv" {
		t.Errorf("login is %q, want %q", got, "kukv")
	}
}

// TestViewerAsksForNoRepository guards the one way this document differs
// from every other query here: it takes no variables, so a backend that
// cannot resolve a repository can still send it.
func TestViewerAsksForNoRepository(t *testing.T) {
	t.Parallel()

	f := &fake{body: []byte(`{"data":{"viewer":{"login":"kukv"}}}`)}
	if _, err := f.client().Viewer(context.Background()); err != nil {
		t.Fatalf("Viewer: %v", err)
	}
	if len(f.vars[0]) != 0 {
		t.Errorf("the query was sent with %d variables, want none", len(f.vars[0]))
	}
	if strings.Contains(f.docs[0], "repository") {
		t.Error("the document names a repository")
	}
}

// TestViewerReportsABodyItCannotRead covers the answer that is not JSON at
// all: a proxy's HTML error page arriving where the API was expected.
func TestViewerReportsABodyItCannotRead(t *testing.T) {
	t.Parallel()

	f := &fake{body: []byte("<html>nope</html>")}
	if _, err := f.client().Viewer(context.Background()); err == nil {
		t.Error("a body that is not JSON was read as a login")
	}
}
