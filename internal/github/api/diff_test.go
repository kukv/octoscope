package api

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// gh pr diff is a GET on the pull request with the diff media type -- verified
// with GH_DEBUG=api on 2026-09-13. Asking for JSON there answers with the pull
// request's fields instead, and the parser would find no files at all.
func TestADiffAsksForTheDiffMediaType(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "pr_diff.txt"))
	})
	files, err := c.PRDiff(context.Background(), "kukv/octoscope", 66)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed")
	}
	req := (*got)[0]
	if a := req.Header.Get("Accept"); a != "application/vnd.github.v3.diff" {
		t.Errorf("Accept = %q", a)
	}
	if req.URL.Path != "/repos/kukv/octoscope/pulls/66" {
		t.Errorf("path = %q", req.URL.Path)
	}
}

// GitHub refuses a diff past a few hundred files; the files API has no such
// limit. The fallback is what makes a large pull request readable at all, so
// a failure on the first call must not end the call.
func TestADiffGitHubRefusesFallsBackToTheFilesApi(t *testing.T) {
	t.Parallel()

	calls := 0
	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = io.WriteString(w, `{"message":"too large"}`)
			return
		}
		_, _ = io.WriteString(w, fixture(t, "pr_files.json"))
	})
	files, err := c.PRDiff(context.Background(), "kukv/octoscope", 66)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("no files parsed from the fallback")
	}
	if (*got)[1].URL.Path != "/repos/kukv/octoscope/pulls/66/files" {
		t.Errorf("fallback path = %q", (*got)[1].URL.Path)
	}
}

// The files API's default page is 30 and one pull request under review had
// 418 files, so a caller that reads only the first page loses most of them.
func TestTheFilesApiFallbackWalksEveryPage(t *testing.T) {
	t.Parallel()

	calls := 0
	var srvURL string
	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		switch calls {
		case 1:
			w.WriteHeader(http.StatusNotAcceptable)
			_, _ = io.WriteString(w, `{"message":"too large"}`)
		case 2:
			w.Header().Set("Link", `<`+srvURL+`/repos/kukv/octoscope/pulls/66/files?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"filename":"a.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@\n-a\n+b"}]`)
		case 3:
			w.Header().Set("Link", `<`+srvURL+`/repos/kukv/octoscope/pulls/66/files?page=3>; rel="next"`)
			_, _ = io.WriteString(w, `[{"filename":"b.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@\n-a\n+b"}]`)
		default:
			_, _ = io.WriteString(w, `[{"filename":"c.go","status":"modified","additions":1,"deletions":0,"patch":"@@ -1 +1 @@\n-a\n+b"}]`)
		}
	})
	srvURL = c.baseURL

	files, err := c.PRDiff(context.Background(), "kukv/octoscope", 66)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("files = %d, want 3 (one per page)", len(files))
	}
	if len(*got) != 4 {
		t.Errorf("requests = %d, want 4 (the refused diff and three pages)", len(*got))
	}
}
