package gql

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/github"
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

// reviewContextHeaderJSON carries the fields the diff view's header
// draws, which arrive with the same query.
const reviewContextHeaderJSON = `{"data":{"repository":{"pullRequest":{
  "id":"PR_1","title":"feat: add relation graph traversal",
  "headRefName":"feat/graph","baseRefName":"main","additions":218,"deletions":31,
  "reviews":{"nodes":[]},"reviewThreads":{"nodes":[]}}}}}`

func TestPRReviewContextBuildsTheQuery(t *testing.T) {
	f := &fake{body: []byte(reviewContextJSON)}
	c := f.client()
	if _, err := c.PRReviewContext(context.Background(), "kukv/koto", 128); err != nil {
		t.Fatal(err)
	}
	for _, want := range []Var{S("owner", "kukv"), S("name", "koto"), N("number", 128)} {
		if !slices.Contains(f.vars[0], want) {
			t.Errorf("vars %v do not carry %v", f.vars[0], want)
		}
	}
}

func TestPRReviewContextReadsTheAnswer(t *testing.T) {
	c := (&fake{body: []byte(reviewContextJSON)}).client()
	rc, err := c.PRReviewContext(context.Background(), "kukv/koto", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.ID != "PR_kwDO1" {
		t.Errorf("id = %q", rc.ID)
	}
	if rc.PendingID != "PRR_kwDO9" {
		t.Errorf("pending id = %q, want the unsubmitted review's", rc.PendingID)
	}
	if len(rc.Threads) != 5 {
		t.Fatalf("%d threads, want the 5 in reviewThreads", len(rc.Threads))
	}

	// What the domain does with a null Line vs. an outdated thread's
	// OriginalLine, and with a thread's DiffSide, is the gateway's
	// toReviewThread's concern (gh/review_test.go); this only checks that
	// the wire fields themselves came out of the JSON as sent.
	tests := []struct {
		name         string
		thread       ReviewThread
		line         *int
		originalLine int
		diffSide     string
	}{
		{"open thread", rc.Threads[0], intPtr(14), 14, "RIGHT"},
		{"resolved thread", rc.Threads[1], intPtr(12), 12, "LEFT"},
		{"outdated thread has no current line", rc.Threads[2], nil, 3, "RIGHT"},
		{"the viewer's unsubmitted comment", rc.Threads[3], intPtr(16), 16, "RIGHT"},
		{"line override distinguishes current from original", rc.Threads[4], intPtr(20), 8, "RIGHT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := tt.thread
			if (th.Line == nil) != (tt.line == nil) || (th.Line != nil && *th.Line != *tt.line) {
				t.Errorf("line = %v, want %v", th.Line, tt.line)
			}
			if th.OriginalLine != tt.originalLine || th.DiffSide != tt.diffSide {
				t.Errorf("originalLine = %d diffSide = %q, want %d %q",
					th.OriginalLine, th.DiffSide, tt.originalLine, tt.diffSide)
			}
		})
	}
}

func intPtr(n int) *int { return &n }

func TestPRReviewContextCarriesTheHeader(t *testing.T) {
	c := (&fake{body: []byte(reviewContextHeaderJSON)}).client()
	rc, err := c.PRReviewContext(context.Background(), "kukv/koto", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.Title != "feat: add relation graph traversal" {
		t.Errorf("title = %q", rc.Title)
	}
	if rc.HeadRefName != "feat/graph" || rc.BaseRefName != "main" {
		t.Errorf("%s -> %s, want feat/graph -> main", rc.HeadRefName, rc.BaseRefName)
	}
	if rc.Additions != 218 || rc.Deletions != 31 {
		t.Errorf("+%d -%d, want +218 -31", rc.Additions, rc.Deletions)
	}
}

func TestPRReviewContextWithNoPendingReview(t *testing.T) {
	c := (&fake{body: []byte(`{"data":{"repository":{"pullRequest":{"id":"PR_1",
		"reviews":{"nodes":[]},"reviewThreads":{"nodes":[]}}}}}`)}).client()
	rc, err := c.PRReviewContext(context.Background(), "kukv/koto", 128)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PendingID != "" {
		t.Errorf("pending id = %q, want empty", rc.PendingID)
	}
}

// TestAThreadCarriesEveryCommentsReviewState guards a thread with more than
// one comment against a decode that stops at the first: whether a comment is
// pending is what the gateway's toReviewThread later decides from this, and
// it needs every comment's own state to do that.
func TestAThreadCarriesEveryCommentsReviewState(t *testing.T) {
	c := (&fake{body: []byte(`{"data":{"repository":{"pullRequest":{
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
}}}}`)}).client()
	rc, err := c.PRReviewContext(context.Background(), "kukv/koto", 128)
	if err != nil {
		t.Fatal(err)
	}
	if len(rc.Threads) != 1 {
		t.Fatalf("%d threads, want 1", len(rc.Threads))
	}
	comments := rc.Threads[0].Comments.Nodes
	if len(comments) != 2 {
		t.Fatalf("%d comments, want 2", len(comments))
	}
	if comments[0].PullRequestReview.State != "COMMENTED" || comments[1].PullRequestReview.State != "PENDING" {
		t.Errorf("states = %q, %q, want COMMENTED then PENDING",
			comments[0].PullRequestReview.State, comments[1].PullRequestReview.State)
	}
}

func TestStartReviewReturnsTheNewReviewID(t *testing.T) {
	f := &fake{body: []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`)}
	c := f.client()
	id, err := c.StartReview(t.Context(), "PR_kwDO1")
	if err != nil {
		t.Fatal(err)
	}
	if id != "PRR_new" {
		t.Errorf("review id = %q, want PRR_new", id)
	}
	if !slices.Contains(f.vars[0], S("pullRequestId", "PR_kwDO1")) {
		t.Errorf("vars %v do not carry the pull request id", f.vars[0])
	}
}

func TestAddReviewThreadSendsTheLineAndTheSide(t *testing.T) {
	tests := []struct {
		name    string
		comment PendingComment
	}{
		{
			name:    "a comment on the new file",
			comment: PendingComment{Path: "graph/walk.go", Line: 15, Side: "RIGHT", Body: "why?"},
		},
		{
			name:    "a comment on a removed line",
			comment: PendingComment{Path: "graph/walk.go", Line: 14, Side: "LEFT", Body: "why?"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fake{body: []byte(`{"data":{"addPullRequestReviewThread":{"thread":{"id":"T_1"}}}}`)}
			c := f.client()
			if err := c.AddReviewThread(t.Context(), "PRR_9", tt.comment); err != nil {
				t.Fatal(err)
			}
			for _, want := range []Var{
				S("reviewId", "PRR_9"),
				S("path", tt.comment.Path),
				N("line", tt.comment.Line),
				S("side", tt.comment.Side),
				S("body", tt.comment.Body),
			} {
				if !slices.Contains(f.vars[0], want) {
					t.Errorf("vars %v do not carry %v", f.vars[0], want)
				}
			}
		})
	}
}

// TestSubmitReviewNamesTheEvent controls the exact strings GraphQL's
// PullRequestReviewEvent enum receives. Do not change these literals without
// checking GitHub's schema: APPROVE versus REQUEST_CHANGES is the difference
// between signing a pull request off and blocking it.
func TestSubmitReviewNamesTheEvent(t *testing.T) {
	tests := []struct {
		name  string
		event ReviewEvent
		want  string
	}{
		{"approve", EventApprove, "APPROVE"},
		{"request changes", EventRequestChanges, "REQUEST_CHANGES"},
		{"comment", EventComment, "COMMENT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fake{body: []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`)}
			c := f.client()
			if err := c.SubmitReview(t.Context(), "PRR_9", tt.event, "looks good"); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(f.vars[0], S("event", tt.want)) {
				t.Errorf("vars %v do not carry event %q", f.vars[0], tt.want)
			}
			if !slices.Contains(f.vars[0], S("body", "looks good")) {
				t.Errorf("vars %v do not carry the body", f.vars[0])
			}
		})
	}
}

func TestSubmitNewReviewCreatesAndSubmitsInOneCall(t *testing.T) {
	f := &fake{body: []byte(`{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`)}
	c := f.client()
	if err := c.SubmitNewReview(t.Context(), "PR_1", EventApprove, ""); err != nil {
		t.Fatal(err)
	}
	for _, want := range []Var{S("pullRequestId", "PR_1"), S("event", "APPROVE"), S("body", "")} {
		if !slices.Contains(f.vars[0], want) {
			t.Errorf("vars %v do not carry %v", f.vars[0], want)
		}
	}
}

func TestDiscardReviewNamesTheReview(t *testing.T) {
	f := &fake{body: []byte(`{"data":{"deletePullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`)}
	c := f.client()
	if err := c.DiscardReview(t.Context(), "PRR_9"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.vars[0], S("reviewId", "PRR_9")) {
		t.Errorf("vars %v do not carry the review id", f.vars[0])
	}
}

func TestABodyThatStartsWithAtIsNotReadAsAFile(t *testing.T) {
	f := &fake{body: []byte(`{"data":{"submitPullRequestReview":{"pullRequestReview":{"id":"PRR_9"}}}}`)}
	c := f.client()
	if err := c.SubmitReview(t.Context(), "PRR_9", EventComment, "@kukv please look"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.vars[0], S("body", "@kukv please look")) {
		t.Errorf("vars %v do not carry the body verbatim", f.vars[0])
	}
}

// A submitted review that was sent twice would post twice. 502 says the
// answer is missing, not that the mutation did not run.
func TestASubmittedReviewIsNeverSentTwice(t *testing.T) {
	t.Parallel()

	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		return nil, github.Classify(github.ErrTransient, "HTTP 502")
	}}
	if err := c.SubmitReview(t.Context(), "R_1", EventApprove, ""); err == nil {
		t.Fatal("SubmitReview succeeded, want an error")
	}
	if calls != 1 {
		t.Errorf("transport called %d times, want 1", calls)
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
	c := (&fake{body: recorded}).client()
	rc, err := c.PRReviewContext(context.Background(), "kukv/octoscope", 55)
	if err != nil {
		t.Fatal(err)
	}
	if rc.PendingID == "" {
		t.Error("no pending review id, but the recording has an unsubmitted review")
	}
	sides := map[string]bool{}
	for _, th := range rc.Threads {
		pending := slices.ContainsFunc(th.Comments.Nodes, func(c ThreadComment) bool {
			return c.PullRequestReview.State == "PENDING"
		})
		if !pending {
			t.Errorf("thread on %s is not pending, but every thread in the recording is", th.Path)
		}
		sides[th.DiffSide] = true
	}
	// Both sides have to survive the parse: the side of a pending thread is
	// the one thing the old query could not ask for.
	if !sides["LEFT"] || !sides["RIGHT"] {
		t.Errorf("threads land on %v, want both sides", sides)
	}
}

// GitHub caps a reviewThreads connection at 100; without a second request
// the extra threads vanish with no error.
func TestPRReviewContextWalksEveryPageOfThreads(t *testing.T) {
	page1 := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":true,"endCursor":"CUR1"},` +
		`"nodes":[{"path":"a.go","originalLine":1,"diffSide":"RIGHT"}]}}}}}`
	page2 := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"CUR2"},` +
		`"nodes":[{"path":"b.go","originalLine":2,"diffSide":"RIGHT"}]}}}}}`

	f := &fakeSeq{outs: []string{page1, page2}}
	c := &Client{Do: f.do}

	rc, err := c.PRReviewContext(t.Context(), "kukv/octoscope", 55)
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
	if !slices.Contains(f.calls[1], S("after", "CUR1")) {
		t.Errorf("second call = %v, want it to carry after=CUR1", f.calls[1])
	}
	// The first request must not: a null cursor is what "from the start"
	// means to GraphQL.
	if slices.ContainsFunc(f.calls[0], func(v Var) bool { return v.Name == "after" }) {
		t.Errorf("first call = %v, want no cursor", f.calls[0])
	}
}

// GitHub caps a thread's comments at what the first page asked for; without
// a second request the rest of a long conversation vanishes with no error.
func TestPRReviewContextWalksEveryPageOfAThreadsComments(t *testing.T) {
	t.Parallel()

	threads := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"T1"},` +
		`"nodes":[{"id":"THREAD_1","path":"a.go","originalLine":1,"diffSide":"RIGHT",` +
		`"comments":{"pageInfo":{"hasNextPage":true,"endCursor":"C50"},` +
		`"nodes":[{"body":"first","author":{"login":"kukv"}}]}}]}}}}}`
	rest := `{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":false,"endCursor":"C99"},` +
		`"nodes":[{"body":"fifty-first","author":{"login":"kukv"}}]}}}}`

	f := &fakeSeq{outs: []string{threads, rest}}
	c := &Client{Do: f.do}

	rc, err := c.PRReviewContext(t.Context(), "kukv/octoscope", 55)
	if err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (the thread says it has more comments)", len(f.calls))
	}
	if !slices.Contains(f.calls[1], S("threadId", "THREAD_1")) {
		t.Errorf("second call = %v, want it to name the thread", f.calls[1])
	}
	// Without the cursor the second request asks for the first fifty again
	// and the loop never ends.
	if !slices.Contains(f.calls[1], S("after", "C50")) {
		t.Errorf("second call = %v, want it to carry after=C50", f.calls[1])
	}
	if len(rc.Threads) != 1 {
		t.Fatalf("Threads = %d, want 1", len(rc.Threads))
	}
	got := rc.Threads[0].Comments.Nodes
	if len(got) != 2 {
		t.Fatalf("Comments = %d, want 2 (one from each page)", len(got))
	}
	if got[0].Body != "first" || got[1].Body != "fifty-first" {
		t.Errorf("Comments = %q / %q, want the second page appended after the first",
			got[0].Body, got[1].Body)
	}
}

func TestAThreadThatFitsInOnePageCostsNoExtraRequest(t *testing.T) {
	t.Parallel()

	threads := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"T1"},` +
		`"nodes":[{"id":"THREAD_1","path":"a.go","originalLine":1,"diffSide":"RIGHT",` +
		`"comments":{"pageInfo":{"hasNextPage":false,"endCursor":"C1"},` +
		`"nodes":[{"body":"only","author":{"login":"kukv"}}]}}]}}}}}`

	f := &fakeSeq{outs: []string{threads}}
	c := &Client{Do: f.do}

	if _, err := c.PRReviewContext(t.Context(), "kukv/octoscope", 55); err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(f.calls) != 1 {
		t.Errorf("calls = %d, want 1: a thread with nothing more must not cost a request", len(f.calls))
	}
}

func TestAThreadWithThreePagesOfCommentsIsFollowedToTheEnd(t *testing.T) {
	t.Parallel()

	threads := `{"data":{"repository":{"pullRequest":{"id":"PR_1",` +
		`"reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":"T1"},` +
		`"nodes":[{"id":"THREAD_1","path":"a.go","originalLine":1,"diffSide":"RIGHT",` +
		`"comments":{"pageInfo":{"hasNextPage":true,"endCursor":"C50"},` +
		`"nodes":[{"body":"one","author":{"login":"kukv"}}]}}]}}}}}`
	page2 := `{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":true,"endCursor":"C150"},` +
		`"nodes":[{"body":"two","author":{"login":"kukv"}}]}}}}`
	page3 := `{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":false,"endCursor":"C250"},` +
		`"nodes":[{"body":"three","author":{"login":"kukv"}}]}}}}`

	f := &fakeSeq{outs: []string{threads, page2, page3}}
	c := &Client{Do: f.do}

	rc, err := c.PRReviewContext(t.Context(), "kukv/octoscope", 55)
	if err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("calls = %d, want 3: no cap is placed on the number of pages", len(f.calls))
	}
	// Without carrying the cursor forward the third call would repeat
	// after=C50 and the walk would loop on the same page forever.
	if !slices.Contains(f.calls[2], S("after", "C150")) {
		t.Errorf("third call = %v, want it to carry after=C150", f.calls[2])
	}
	if len(rc.Threads[0].Comments.Nodes) != 3 {
		t.Errorf("Comments = %d, want 3", len(rc.Threads[0].Comments.Nodes))
	}
}
