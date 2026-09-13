package gh

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

func (f fakeBackend) PRDiff(ctx context.Context, repo string, number int) (github.Diff, error) {
	return f.prDiff(ctx, repo, number)
}

func (f fakeBackend) PRReviewContext(ctx context.Context, repo string, number int) (gql.ReviewContext, error) {
	return f.prReviewContext(ctx, repo, number)
}

func (f fakeBackend) AddReviewThread(reviewID string, c gql.PendingComment) error {
	return f.addReviewThread(reviewID, c)
}

func (f fakeBackend) SubmitReview(reviewID string, event gql.ReviewEvent, body string) error {
	return f.submitReview(reviewID, event, body)
}

func (f fakeBackend) SubmitNewReview(pullRequestID string, event gql.ReviewEvent, body string) error {
	return f.submitNewReview(pullRequestID, event, body)
}

// TestFileStatusFromAPIMapsEveryValue guards the files API's status
// spelling, including GitHub saying "removed" rather than "deleted".
func TestFileStatusFromAPIMapsEveryValue(t *testing.T) {
	tests := []struct {
		api  string
		want domain.FileStatus
	}{
		{"added", domain.FileAdded},
		{"removed", domain.FileDeleted},
		{"modified", domain.FileModified},
		{"renamed", domain.FileRenamed},
		{"copied", domain.FileCopied},
		{"changed", domain.FileChanged},
		{"unchanged", domain.FileUnchanged},
	}
	for _, tt := range tests {
		t.Run(tt.api, func(t *testing.T) {
			if got := toFileStatus(tt.api); got != tt.want {
				t.Errorf("toFileStatus(%q) = %v, want %v", tt.api, got, tt.want)
			}
		})
	}
}

// TestPRDiffFromRawUsesParseDiff covers the common path: gh pr diff or the
// REST diff media type answered, so the backend hands back raw unified diff
// text and the gateway parses it with domain.ParseDiff.
func TestPRDiffFromRawUsesParseDiff(t *testing.T) {
	t.Parallel()

	raw := []byte("diff --git a/x.go b/x.go\n" +
		"@@ -1,1 +1,1 @@\n-old\n+new\n")

	g := New(fakeBackend{prDiff: func(context.Context, string, int) (github.Diff, error) {
		return github.Diff{Raw: raw}, nil
	}})

	got, err := g.PRDiff(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}
	want := domain.ParseDiff(raw)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PRDiff() = %+v, want the same as domain.ParseDiff(raw): %+v", got, want)
	}
}

// TestPRDiffFromFilesTranslatesTheWireShapeIntoTheDomain gives every field
// of github.PRFile a distinct, non-zero value (Patch set in one case, nil in
// the other, since the two are mutually exclusive) and compares the whole
// resulting domain.FileDiff against a fully written-out expectation. Binary
// is not settable here: the files API has no way to say a file is binary,
// so it stays false in both fixtures -- 7 of FileDiff's 8 fields are guarded
// by this test, and Binary is guarded instead by domain.ParseDiff's own
// tests.
func TestPRDiffFromFilesTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	patch := "@@ -3,1 +3,1 @@\n-old line\n+new line"

	g := New(fakeBackend{prDiff: func(context.Context, string, int) (github.Diff, error) {
		return github.Diff{Files: []github.PRFile{
			{
				Filename:         "b.md",
				PreviousFilename: "a.md",
				Status:           "renamed",
				Additions:        5,
				Deletions:        3,
				Patch:            &patch,
			},
			{
				Filename:         "huge.bin",
				PreviousFilename: "",
				Status:           "modified",
				Additions:        0,
				Deletions:        0,
				Patch:            nil,
			},
		}}, nil
	}})

	got, err := g.PRDiff(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatalf("PRDiff: %v", err)
	}

	want := []domain.FileDiff{
		{
			Path:      "b.md",
			OldPath:   "a.md",
			Status:    domain.FileRenamed,
			Additions: 5,
			Deletions: 3,
			Hunks: []domain.Hunk{
				{
					Header: "@@ -3,1 +3,1 @@",
					Lines: []domain.DiffLine{
						{Kind: domain.LineRemoved, OldLine: 3, Text: "old line"},
						{Kind: domain.LineAdded, NewLine: 3, Text: "new line"},
					},
				},
			},
		},
		{
			Path:         "huge.bin",
			Status:       domain.FileModified,
			PatchOmitted: true,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PRDiff() = %+v, want %+v", got, want)
	}
}

// TestPRReviewContextTranslatesTheWireShapeIntoTheDomain gives every field of
// gql.ReviewContext and its threads a distinct, non-zero value and compares
// the whole resulting domain.ReviewContext against a fully written-out
// expectation. The two threads differ in the fields toReviewThread has to
// choose between: one has a current Line distinct from its OriginalLine
// (guarding against the two being swapped), the other has none and must
// fall back to OriginalLine; one is on each DiffSide.
func TestPRReviewContextTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	commentA := time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC)
	commentB := time.Date(2026, 9, 11, 10, 0, 0, 0, time.UTC)
	currentLine := 21

	rc := gql.ReviewContext{
		ID:          "PR_1",
		Title:       "add the gateway",
		HeadRefName: "refactor/pr2b-gateway",
		BaseRefName: "main",
		Additions:   42,
		Deletions:   7,
		PendingID:   "PRR_9",
	}
	threadA := gql.ReviewThread{
		ID:           "THREAD_A",
		IsResolved:   true,
		IsOutdated:   false,
		Path:         "a.go",
		Line:         &currentLine,
		OriginalLine: 5,
		DiffSide:     "LEFT",
	}
	threadA.Comments.Nodes = []gql.ThreadComment{{
		Body:      "hi",
		CreatedAt: commentA,
		Author:    gql.Author{Login: "kukv"},
	}}
	threadA.Comments.Nodes[0].PullRequestReview.State = "PENDING"

	threadB := gql.ReviewThread{
		ID:           "THREAD_B",
		IsResolved:   false,
		IsOutdated:   true,
		Path:         "b.go",
		Line:         nil,
		OriginalLine: 9,
		DiffSide:     "RIGHT",
	}
	threadB.Comments.Nodes = []gql.ThreadComment{{
		Body:      "yo",
		CreatedAt: commentB,
		Author:    gql.Author{Login: "other"},
	}}
	threadB.Comments.Nodes[0].PullRequestReview.State = "COMMENTED"

	rc.Threads = []gql.ReviewThread{threadA, threadB}

	g := New(fakeBackend{prReviewContext: func(context.Context, string, int) (gql.ReviewContext, error) {
		return rc, nil
	}})

	got, err := g.PRReviewContext(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatalf("PRReviewContext: %v", err)
	}

	want := domain.ReviewContext{
		PullRequestID: "PR_1",
		Title:         "add the gateway",
		Head:          "refactor/pr2b-gateway",
		Base:          "main",
		Additions:     42,
		Deletions:     7,
		PendingID:     "PRR_9",
		Threads: []domain.ReviewThread{
			{
				Path:     "a.go",
				Line:     21,
				Side:     domain.SideLeft,
				Resolved: true,
				Outdated: false,
				Comments: []domain.ThreadComment{
					{Author: domain.Author{Login: "kukv"}, Body: "hi", CreatedAt: commentA, Pending: true},
				},
			},
			{
				Path:     "b.go",
				Line:     9,
				Side:     domain.SideRight,
				Resolved: false,
				Outdated: true,
				Comments: []domain.ThreadComment{
					{Author: domain.Author{Login: "other"}, Body: "yo", CreatedAt: commentB, Pending: false},
				},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PRReviewContext() = %+v, want %+v", got, want)
	}
}

// TestFromReviewEventMapsEveryValue guards the only place that decides what
// PullRequestReviewEvent string a review's event turns into. APPROVE versus
// REQUEST_CHANGES is the difference between signing a pull request off and
// blocking it, and a wrong mapping submits the wrong one to GitHub without
// ever failing, so every domain.ReviewEvent value is covered here.
func TestFromReviewEventMapsEveryValue(t *testing.T) {
	tests := []struct {
		event domain.ReviewEvent
		want  gql.ReviewEvent
	}{
		{domain.EventComment, gql.EventComment},
		{domain.EventApprove, gql.EventApprove},
		{domain.EventRequestChanges, gql.EventRequestChanges},
	}
	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			if got := fromReviewEvent(tt.event); got != tt.want {
				t.Errorf("fromReviewEvent(%v) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

// TestFromDiffSideMapsBothValues covers both domain.DiffSide values.
func TestFromDiffSideMapsBothValues(t *testing.T) {
	tests := []struct {
		side domain.DiffSide
		want string
	}{
		{domain.SideLeft, "LEFT"},
		{domain.SideRight, "RIGHT"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := fromDiffSide(tt.side); got != tt.want {
				t.Errorf("fromDiffSide(%v) = %q, want %q", tt.side, got, tt.want)
			}
		})
	}
}

// TestFromPendingCommentTranslatesEveryField gives every field of
// domain.PendingComment a distinct, non-zero value and compares the whole
// resulting gql.PendingComment against a fully written-out expectation.
func TestFromPendingCommentTranslatesEveryField(t *testing.T) {
	t.Parallel()

	c := domain.PendingComment{
		Path: "graph/walk.go",
		Line: 42,
		Side: domain.SideLeft,
		Body: "why?",
	}
	got := fromPendingComment(c)
	want := gql.PendingComment{
		Path: "graph/walk.go",
		Line: 42,
		Side: "LEFT",
		Body: "why?",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fromPendingComment() = %+v, want %+v", got, want)
	}
}

// TestAddReviewThreadConvertsThePendingCommentBeforeCallingTheBackend checks
// that the gateway hands the backend the converted wire value, not the
// domain one.
func TestAddReviewThreadConvertsThePendingCommentBeforeCallingTheBackend(t *testing.T) {
	t.Parallel()

	var got gql.PendingComment
	g := New(fakeBackend{addReviewThread: func(reviewID string, c gql.PendingComment) error {
		got = c
		return nil
	}})

	in := domain.PendingComment{Path: "a.go", Line: 3, Side: domain.SideRight, Body: "hi"}
	if err := g.AddReviewThread("PRR_1", in); err != nil {
		t.Fatalf("AddReviewThread: %v", err)
	}

	want := gql.PendingComment{Path: "a.go", Line: 3, Side: "RIGHT", Body: "hi"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("backend received %+v, want %+v", got, want)
	}
}

// TestSubmitReviewConvertsTheEventBeforeCallingTheBackend checks that the
// gateway hands the backend the converted wire event, not the domain one.
func TestSubmitReviewConvertsTheEventBeforeCallingTheBackend(t *testing.T) {
	t.Parallel()

	var gotEvent gql.ReviewEvent
	g := New(fakeBackend{submitReview: func(reviewID string, event gql.ReviewEvent, body string) error {
		gotEvent = event
		return nil
	}})

	if err := g.SubmitReview("PRR_1", domain.EventApprove, "lgtm"); err != nil {
		t.Fatalf("SubmitReview: %v", err)
	}
	if gotEvent != gql.EventApprove {
		t.Errorf("backend received event %v, want %v", gotEvent, gql.EventApprove)
	}
}

// TestSubmitNewReviewConvertsTheEventBeforeCallingTheBackend checks that the
// gateway hands the backend the converted wire event, not the domain one.
func TestSubmitNewReviewConvertsTheEventBeforeCallingTheBackend(t *testing.T) {
	t.Parallel()

	var gotEvent gql.ReviewEvent
	g := New(fakeBackend{submitNewReview: func(pullRequestID string, event gql.ReviewEvent, body string) error {
		gotEvent = event
		return nil
	}})

	if err := g.SubmitNewReview("PR_1", domain.EventRequestChanges, ""); err != nil {
		t.Fatalf("SubmitNewReview: %v", err)
	}
	if gotEvent != gql.EventRequestChanges {
		t.Errorf("backend received event %v, want %v", gotEvent, gql.EventRequestChanges)
	}
}
