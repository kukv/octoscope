package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

const reviewContextJSON = `{"data":{"repository":{"pullRequest":{
  "id":"PR_kwDO1",
  "reviews":{"nodes":[{"id":"PRR_kwDO9"}]},
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
	if len(rc.Threads) != 5 {
		t.Fatalf("%d threads, want the 5 in reviewThreads", len(rc.Threads))
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

// TestPRReviewContextRejectsARepoWithNoSlash guards against silently querying
// the wrong repository: a --repo value with no "/" cannot be split into
// owner and name, so the call must fail rather than send an empty owner or
// name to GitHub.
func TestPRReviewContextRejectsARepoWithNoSlash(t *testing.T) {
	c := New("/w", "not-a-repo")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("gh was invoked with a repo that cannot be split into owner/name")
		return nil, nil
	}
	if _, err := c.PRReviewContext(context.Background(), "", 128); err == nil {
		t.Fatal("PRReviewContext did not fail for a repo with no slash")
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

func TestStartReviewReturnsTheNewReviewID(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`), nil
	}
	id, err := c.StartReview("PR_kwDO1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "PRR_new" {
		t.Errorf("review id = %q, want PRR_new", id)
	}
	if !slices.Contains(got, "pullRequestId=PR_kwDO1") {
		t.Errorf("args %v do not carry the pull request id", got)
	}
}

func TestAddReviewThreadSendsTheLineAndTheSide(t *testing.T) {
	tests := []struct {
		name    string
		comment gh.PendingComment
		side    string
	}{
		{
			name:    "a comment on the new file",
			comment: gh.PendingComment{Path: "graph/walk.go", Line: 15, Side: gh.SideRight, Body: "why?"},
			side:    "side=RIGHT",
		},
		{
			name:    "a comment on a removed line",
			comment: gh.PendingComment{Path: "graph/walk.go", Line: 14, Side: gh.SideLeft, Body: "why?"},
			side:    "side=LEFT",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("/w", "kukv/koto")
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(`{"data":{"addPullRequestReviewThread":{"thread":{"id":"T_1"}}}}`), nil
			}
			if err := c.AddReviewThread("PRR_9", tt.comment); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{
				"reviewId=PRR_9",
				"path=" + tt.comment.Path,
				"line=" + strconv.Itoa(tt.comment.Line),
				tt.side,
				"body=" + tt.comment.Body,
			} {
				if !slices.Contains(got, want) {
					t.Errorf("args %v do not carry %q", got, want)
				}
			}
		})
	}
}

func TestSubmitReviewNamesTheEvent(t *testing.T) {
	tests := []struct {
		name  string
		event gh.ReviewEvent
		want  string
	}{
		{"approve", gh.EventApprove, "event=APPROVE"},
		{"request changes", gh.EventRequestChanges, "event=REQUEST_CHANGES"},
		{"comment", gh.EventComment, "event=COMMENT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New("/w", "kukv/koto")
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`), nil
			}
			if err := c.SubmitReview("PRR_9", tt.event, "looks good"); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(got, tt.want) {
				t.Errorf("args %v do not carry %q", got, tt.want)
			}
			if !slices.Contains(got, "body=looks good") {
				t.Errorf("args %v do not carry the body", got)
			}
		})
	}
}

func TestSubmitNewReviewCreatesAndSubmitsInOneCall(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`), nil
	}
	if err := c.SubmitNewReview("PR_1", gh.EventApprove, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"pullRequestId=PR_1", "event=APPROVE", "body="} {
		if !slices.Contains(got, want) {
			t.Errorf("args %v do not carry %q", got, want)
		}
	}
}

func TestDiscardReviewNamesTheReview(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"deletePullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`), nil
	}
	if err := c.DiscardReview("PRR_9"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "reviewId=PRR_9") {
		t.Errorf("args %v do not carry the review id", got)
	}
}

func TestABodyThatStartsWithAtIsNotReadAsAFile(t *testing.T) {
	c := New("/w", "kukv/koto")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`), nil
	}
	if err := c.SubmitReview("PRR_9", gh.EventComment, "@kukv please look"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got, "body=@kukv please look") {
		t.Errorf("args %v do not carry the body verbatim", got)
	}
}

// TestPRReviewContextParsesTheRecordedAnswer runs the parse over what GitHub
// actually sent, rather than over JSON written to match the struct. The
// recording carries an unsubmitted review with one thread on each side; see
// testdata/README.md.
func TestPRReviewContextParsesTheRecordedAnswer(t *testing.T) {
	recorded, err := os.ReadFile("testdata/review_context.json")
	if err != nil {
		t.Fatal(err)
	}
	c := New("/w", "kukv/octoscope")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return recorded, nil
	}
	rc, err := c.PRReviewContext(context.Background(), "", 55)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PendingID == "" {
		t.Error("no pending review id, but the recording has an unsubmitted review")
	}
	sides := map[gh.DiffSide]bool{}
	for _, th := range rc.Threads {
		if !th.Pending() {
			t.Errorf("thread on %s:%d is not pending, but every thread in the recording is", th.Path, th.Line)
		}
		sides[th.Side] = true
	}
	// Both sides have to survive the parse: the side of a pending thread is
	// the one thing the old query could not ask for.
	if !sides[gh.SideLeft] || !sides[gh.SideRight] {
		t.Errorf("threads land on %v, want both sides", sides)
	}
}

// fakeSeq answers each call with the next canned output, so a test can watch
// a paging loop walk more than one page. It records every argument list.
type fakeSeq struct {
	outs  []string
	calls [][]string
}

func (f *fakeSeq) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	i := len(f.calls) - 1
	if i >= len(f.outs) {
		return nil, fmt.Errorf("unexpected call %d: %v", i, args)
	}
	return []byte(f.outs[i]), nil
}

// GraphQL connections cap first at 100 (101 is refused with
// EXCESSIVE_PAGINATION), so a pull request with more review threads than
// that needs a second request. Without one the extra threads vanish with no
// error, which is the failure mode this whole remediation is about.
func TestPRReviewContextWalksEveryPageOfThreads(t *testing.T) {
	page1 := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":true,"endCursor":"CUR1"},` +
		`"nodes":[{"path":"a.go","originalLine":1,"diffSide":"RIGHT"}]}}}}}`
	page2 := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"CUR2"},` +
		`"nodes":[{"path":"b.go","originalLine":2,"diffSide":"RIGHT"}]}}}}}`

	f := &fakeSeq{outs: []string{page1, page2}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	rc, err := c.PRReviewContext(t.Context(), "", 55)
	if err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(rc.Threads) != 2 {
		t.Fatalf("threads = %d, want 2 (both pages)", len(rc.Threads))
	}
	if rc.Threads[0].Path != "a.go" || rc.Threads[1].Path != "b.go" {
		t.Errorf("threads = %+v, want a.go then b.go in order", rc.Threads)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(f.calls))
	}
	// The second request has to carry the first page's cursor, or it just
	// asks for page one again and the loop never ends.
	if !slices.Contains(f.calls[1], "after=CUR1") {
		t.Errorf("second call = %v, want it to carry after=CUR1", f.calls[1])
	}
	// The first request must not: a null cursor is what "from the start"
	// means to GraphQL.
	if slices.ContainsFunc(f.calls[0], func(s string) bool {
		return strings.HasPrefix(s, "after=")
	}) {
		t.Errorf("first call = %v, want no cursor", f.calls[0])
	}
}
