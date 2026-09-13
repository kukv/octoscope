package gh

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// fakeCrossRepoLister answers SearchItems and RepoCounts with whatever a
// test set. Embedding the nil backend panics loudly if a test calls a method
// it did not mean to exercise.
type fakeCrossRepoLister struct {
	backend
	searchItems func(ctx context.Context, query string) ([]gql.SearchItem, error)
	repoCounts  func(ctx context.Context, repos []string) ([]gql.RepoCount, error)
}

func (f fakeCrossRepoLister) SearchItems(ctx context.Context, query string) ([]gql.SearchItem, error) {
	return f.searchItems(ctx, query)
}

func (f fakeCrossRepoLister) RepoCounts(ctx context.Context, repos []string) ([]gql.RepoCount, error) {
	return f.repoCounts(ctx, repos)
}

// TestWorkQueryDefinesEachColumn pins the four search strings the Work board
// is built on. They are copied here from what internal/github/gql/search.go
// used to build them from, not from workQuery's own output: a test that
// copied the implementation would pass even if a column's meaning changed.
func TestWorkQueryDefinesEachColumn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		section domain.WorkSection
		want    string
	}{
		{domain.SectionReviewRequested, "is:open is:pr review-requested:@me"},
		{domain.SectionYourPRs, "is:open is:pr author:@me"},
		{domain.SectionAssigned, "is:open assignee:@me"},
		{domain.SectionMentioned, "is:open mentions:@me"},
	}
	if len(tests) != domain.WorkSectionCount {
		t.Fatalf("the table covers %d columns, the board has %d", len(tests), domain.WorkSectionCount)
	}
	for _, tt := range tests {
		if got := workQuery(tt.section); got != tt.want {
			t.Errorf("workQuery(%v) = %q, want %q", tt.section, got, tt.want)
		}
	}
}

// TestListWorkSectionSendsTheColumnsQuery guards the wiring between
// ListWorkSection and workQuery: a column asked for must run its own search,
// not another one.
func TestListWorkSectionSendsTheColumnsQuery(t *testing.T) {
	t.Parallel()

	var gotQuery string
	g := New(fakeCrossRepoLister{searchItems: func(_ context.Context, query string) ([]gql.SearchItem, error) {
		gotQuery = query
		return nil, nil
	}})

	if _, err := g.ListWorkSection(context.Background(), domain.SectionMentioned); err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	if want := "is:open mentions:@me"; gotQuery != want {
		t.Errorf("ListWorkSection sent %q, want %q", gotQuery, want)
	}
}

// TestListWorkSectionRejectsASectionTheBoardDoesNotHave guards against a bug
// in the caller reaching the backend as a search: a section outside the
// board must fail here rather than index workQueries out of range.
func TestListWorkSectionRejectsASectionTheBoardDoesNotHave(t *testing.T) {
	t.Parallel()

	g := New(fakeCrossRepoLister{searchItems: func(context.Context, string) ([]gql.SearchItem, error) {
		t.Error("an unknown section was sent to the backend")
		return nil, nil
	}})

	if _, err := g.ListWorkSection(context.Background(), domain.WorkSectionCount); err == nil {
		t.Error("ListWorkSection accepted a section the board does not have")
	}
}

// TestSearchItemsSendsTheQueryItWasGiven guards that the Search tab's own
// query reaches the backend unchanged, unlike a Work board column.
func TestSearchItemsSendsTheQueryItWasGiven(t *testing.T) {
	t.Parallel()

	var gotQuery string
	g := New(fakeCrossRepoLister{searchItems: func(_ context.Context, query string) ([]gql.SearchItem, error) {
		gotQuery = query
		return nil, nil
	}})

	if _, err := g.SearchItems(context.Background(), "repo:kukv/octoscope is:pr"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if want := "repo:kukv/octoscope is:pr"; gotQuery != want {
		t.Errorf("SearchItems sent %q, want %q", gotQuery, want)
	}
}

// TestToWorkItemTranslatesTheWireShapeIntoTheDomain gives every field of
// gql.SearchItem a distinct, non-zero value and compares the whole resulting
// domain.WorkItem against a fully written-out expectation. A field the
// conversion forgot to copy is left at its zero value, which a struct-wide
// comparison catches; asserting a handful of fields would not.
func TestToWorkItemTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	updatedAt := time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)

	node := gql.SearchItem{
		Typename:       "PullRequest",
		Number:         59,
		Title:          "build the queries in the gateway",
		State:          "MERGED",
		URL:            "https://github.com/kukv/octoscope/pull/59",
		IsDraft:        true,
		BodyText:       "the section knows the query now",
		UpdatedAt:      updatedAt,
		ReviewDecision: "CHANGES_REQUESTED",
		HeadRefName:    "refactor/pr2b-gateway",
		BaseRefName:    "main",
		Additions:      42,
		Deletions:      7,
		Author:         gql.Author{Login: "kukv"},
	}
	node.Repository.NameWithOwner = "kukv/octoscope"
	node.Labels.Nodes = []gql.Label{
		{Name: "bug", Color: "d73a4a"},
		{Name: "wip", Color: "ededed"},
	}
	// node.Commits.Nodes is an anonymous struct too deeply nested to build
	// field by field, so it is filled the way the real client fills it: by
	// decoding it out of JSON in the shape the roll-up query returns.
	const commits = `[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
		{"__typename":"CheckRun","name":"build","status":"COMPLETED","conclusion":"SUCCESS"},
		{"__typename":"StatusContext","context":"ci/deploy","state":"FAILURE"}
	]}}}}]`
	if err := json.Unmarshal([]byte(commits), &node.Commits.Nodes); err != nil {
		t.Fatalf("build commits fixture: %v", err)
	}

	got := toWorkItem(node)

	want := domain.WorkItem{
		Ref: domain.ItemRef{
			Kind:   domain.ItemPR,
			Repo:   "kukv/octoscope",
			Number: 59,
		},
		Title:   "build the queries in the gateway",
		Body:    "the section knows the query now",
		Author:  "kukv",
		IsDraft: true,
		State:   domain.StateMerged,
		Labels: []domain.Label{
			{Name: "bug", Color: "d73a4a"},
			{Name: "wip", Color: "ededed"},
		},
		Review: domain.ReviewChangesRequested,
		Head:   "refactor/pr2b-gateway",
		Base:   "main",
		Checks: domain.Checks{
			Total:  2,
			Passed: 1,
			Failed: 1,
			State:  domain.CheckFailure,
			Runs: []domain.CheckRun{
				{Name: "build", State: domain.CheckSuccess, Kind: domain.CheckKindRun},
				{Name: "ci/deploy", State: domain.CheckFailure, Kind: domain.CheckKindStatus},
			},
		},
		Additions: 42,
		Deletions: 7,
		UpdatedAt: updatedAt,
		URL:       "https://github.com/kukv/octoscope/pull/59",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toWorkItem() = %+v, want %+v", got, want)
	}
}

// TestToWorkItemLeavesPullRequestOnlyFieldsZeroForAnIssue guards the other
// branch: an issue has no Head, Base, size, draft state, review or checks.
func TestToWorkItemLeavesPullRequestOnlyFieldsZeroForAnIssue(t *testing.T) {
	t.Parallel()

	node := gql.SearchItem{
		Typename: "Issue",
		Number:   7,
		Title:    "track the conversion",
		State:    "CLOSED",
		Author:   gql.Author{Login: "kukv"},
	}
	node.Repository.NameWithOwner = "kukv/octoscope"

	got := toWorkItem(node)

	want := domain.WorkItem{
		Ref: domain.ItemRef{
			Kind:   domain.ItemIssue,
			Repo:   "kukv/octoscope",
			Number: 7,
		},
		Title:  "track the conversion",
		Author: "kukv",
		State:  domain.StateClosed,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toWorkItem() = %+v, want %+v", got, want)
	}
}

// TestToRepoCountTranslatesTheWireShapeIntoTheDomain gives every field of
// gql.RepoCount a distinct, non-zero value and compares the whole resulting
// domain.RepoCount against a fully written-out expectation, for the same
// reason as TestToWorkItemTranslatesTheWireShapeIntoTheDomain above.
func TestToRepoCountTranslatesTheWireShapeIntoTheDomain(t *testing.T) {
	t.Parallel()

	got := toRepoCount(gql.RepoCount{
		Repo:        "kukv/octoscope",
		PRs:         3,
		Issues:      5,
		Unavailable: true,
	})

	want := domain.RepoCount{
		Repo:        "kukv/octoscope",
		PRs:         3,
		Issues:      5,
		Unavailable: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toRepoCount() = %+v, want %+v", got, want)
	}
}

// TestRepoCountsTranslatesEveryNode guards the loop around toRepoCount: each
// repository's own row must come back, in order.
func TestRepoCountsTranslatesEveryNode(t *testing.T) {
	t.Parallel()

	g := New(fakeCrossRepoLister{repoCounts: func(context.Context, []string) ([]gql.RepoCount, error) {
		return []gql.RepoCount{
			{Repo: "kukv/octoscope", PRs: 1, Issues: 2},
			{Repo: "cli/cli", Unavailable: true},
		}, nil
	}})

	got, err := g.RepoCounts(context.Background(), []string{"kukv/octoscope", "cli/cli"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	want := []domain.RepoCount{
		{Repo: "kukv/octoscope", PRs: 1, Issues: 2},
		{Repo: "cli/cli", Unavailable: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RepoCounts() = %+v, want %+v", got, want)
	}
}
