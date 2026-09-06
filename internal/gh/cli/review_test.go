package cli

import (
	"context"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

const reviewContextJSON = `{"data":{"repository":{"pullRequest":{
  "id":"PR_kwDO1",
  "reviews":{"nodes":[{"id":"PRR_kwDO9","comments":{"nodes":[
    {"path":"graph/walk.go","line":16,"diffSide":"RIGHT","body":"duplicate pending at existing position"},
    {"path":"graph/new.go","line":42,"diffSide":"RIGHT","body":"pending at new position"}
  ]}}]},
  "reviewThreads":{"nodes":[
    {"isResolved":false,"isOutdated":false,"path":"graph/walk.go","line":14,
     "originalLine":14,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"is 2 not the default here?","createdAt":"2026-09-06T12:00:00Z",
        "author":{"login":"kukv"},"pullRequestReview":{"state":"COMMENTED"}}]}},
    {"isResolved":true,"isOutdated":false,"path":"graph/walk.go","line":12,
     "originalLine":12,"diffSide":"LEFT","comments":{"nodes":[
       {"body":"settled","createdAt":"2026-09-05T12:00:00Z",
        "author":{"login":"someone"},"pullRequestReview":{"state":"APPROVED"}}]}},
    {"isResolved":false,"isOutdated":true,"path":"graph/old.go","line":null,
     "originalLine":3,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"moved since","createdAt":"2026-09-04T12:00:00Z",
        "author":{"login":"someone"},"pullRequestReview":{"state":"COMMENTED"}}]}},
    {"isResolved":false,"isOutdated":false,"path":"graph/walk.go","line":16,
     "originalLine":16,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"mine, not sent yet","createdAt":"2026-09-06T13:00:00Z",
        "author":{"login":"kukv"},"pullRequestReview":{"state":"PENDING"}}]}},
    {"isResolved":false,"isOutdated":false,"path":"graph/moved.go","line":20,
     "originalLine":8,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"code moved slightly","createdAt":"2026-09-06T11:00:00Z",
        "author":{"login":"someone"},"pullRequestReview":{"state":"COMMENTED"}}]}}
  ]}
}}}}`

// reviewContextJSONWithHeader carries the fields the diff view's header
// draws, which arrive with the same query.
const reviewContextHeaderJSON = `{"data":{"repository":{"pullRequest":{
  "id":"PR_1","title":"feat: add relation graph traversal",
  "headRefName":"feat/graph","baseRefName":"main","additions":218,"deletions":31,
  "reviews":{"nodes":[]},"reviewThreads":{"nodes":[]}}}}}`

func TestPRReviewContextBuildsTheQuery(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(reviewContextJSON), nil
	}
	if _, err := c.PRReviewContext(context.Background(), "", 128); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"api", "graphql", "-f", "owner=kukv", "-f", "name=koto", "-F", "number=128"} {
		if !slices.Contains(got, want) {
			t.Errorf("args %v do not carry %q", got, want)
		}
	}
}

func TestPRReviewContextReadsTheAnswer(t *testing.T) {
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(reviewContextJSON), nil
	}
	rc, err := c.PRReviewContext(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PullRequestID != "PR_kwDO1" {
		t.Errorf("pull request id = %q", rc.PullRequestID)
	}
	if rc.PendingID != "PRR_kwDO9" {
		t.Errorf("pending id = %q, want the unsubmitted review's", rc.PendingID)
	}
	if len(rc.Threads) != 6 {
		t.Fatalf("%d threads, want 6 (5 from reviewThreads + 1 appended pending comment)", len(rc.Threads))
	}

	tests := []struct {
		name      string
		thread    gh.ReviewThread
		line      int
		side      gh.DiffSide
		collapsed bool
		pending   bool
	}{
		{"open thread", rc.Threads[0], 14, gh.SideRight, false, false},
		{"resolved threads collapse", rc.Threads[1], 12, gh.SideLeft, true, false},
		{"outdated threads keep the line they were written against", rc.Threads[2], 3, gh.SideRight, true, false},
		{"the viewer's unsubmitted comment", rc.Threads[3], 16, gh.SideRight, false, true},
		{"line override distinguishes current from original", rc.Threads[4], 20, gh.SideRight, false, false},
		{"pending comment appended from review.comments", rc.Threads[5], 42, gh.SideRight, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := tt.thread
			if th.Line != tt.line || th.Side != tt.side {
				t.Errorf("line %d side %v, want %d %v", th.Line, th.Side, tt.line, tt.side)
			}
			if th.Collapsed() != tt.collapsed {
				t.Errorf("Collapsed() = %v, want %v", th.Collapsed(), tt.collapsed)
			}
			if th.Pending() != tt.pending {
				t.Errorf("Pending() = %v, want %v", th.Pending(), tt.pending)
			}
		})
	}
}

// TestPRReviewContextInTheWorkingDirectorysRepo is the ordinary case: no
// --repo, so there is no "owner/name" to split and gh has to fill the
// placeholders from the checkout's remote. Sending empty strings here asks
// GitHub for a repository called "".
func TestPRReviewContextCarriesTheHeader(t *testing.T) {
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(reviewContextHeaderJSON), nil
	}
	rc, err := c.PRReviewContext(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.Title != "feat: add relation graph traversal" {
		t.Errorf("title = %q", rc.Title)
	}
	if rc.Head != "feat/graph" || rc.Base != "main" {
		t.Errorf("%s -> %s, want feat/graph -> main", rc.Head, rc.Base)
	}
	if rc.Additions != 218 || rc.Deletions != 31 {
		t.Errorf("+%d -%d, want +218 -31", rc.Additions, rc.Deletions)
	}
}

func TestPRReviewContextInTheWorkingDirectorysRepo(t *testing.T) {
	c := New("/w", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(reviewContextJSON), nil
	}
	if _, err := c.PRReviewContext(context.Background(), "", 128); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"owner={owner}", "name={repo}"} {
		if !slices.Contains(got, want) {
			t.Errorf("args %v do not carry %q", got, want)
		}
	}
	for _, unwanted := range []string{"owner=", "name="} {
		if slices.Contains(got, unwanted) {
			t.Errorf("args %v name an empty repository", got)
		}
	}
}

func TestPRReviewContextWithNoPendingReview(t *testing.T) {
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_1",
			"reviews":{"nodes":[]},"reviewThreads":{"nodes":[]}}}}}`), nil
	}
	rc, err := c.PRReviewContext(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PendingID != "" {
		t.Errorf("pending id = %q, want empty", rc.PendingID)
	}
}

func TestThreadWithPublicThenPendingCommentReportsPending(t *testing.T) {
	c := New("/w", "kukv/koto")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"data":{"repository":{"pullRequest":{
  "id":"PR_1",
  "reviews":{"nodes":[]},
  "reviewThreads":{"nodes":[
    {"isResolved":false,"isOutdated":false,"path":"graph/walk.go","line":10,
     "originalLine":10,"diffSide":"RIGHT","comments":{"nodes":[
       {"body":"public comment","createdAt":"2026-09-06T12:00:00Z",
        "author":{"login":"someone"},"pullRequestReview":{"state":"COMMENTED"}},
       {"body":"pending reply","createdAt":"2026-09-06T13:00:00Z",
        "author":{"login":"kukv"},"pullRequestReview":{"state":"PENDING"}}]}}
  ]}
}}}}`), nil
	}
	rc, err := c.PRReviewContext(context.Background(), "", 128)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Threads) != 1 {
		t.Fatalf("%d threads, want 1", len(rc.Threads))
	}
	if !rc.Threads[0].Pending() {
		t.Errorf("Pending() = false, want true for thread with pending reply")
	}
}
