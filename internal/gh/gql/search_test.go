package gql

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

const workJSON = `{"data":{"results":{"nodes":[
  {"__typename":"PullRequest","number":12,"title":"fix the thing",
   "url":"https://github.com/kukv/octoscope/pull/12","isDraft":false,
   "updatedAt":"2026-09-06T12:00:00Z","reviewDecision":"REVIEW_REQUIRED",
   "author":{"login":"someone"},
   "repository":{"nameWithOwner":"kukv/octoscope"},
   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
      {"__typename":"CheckRun","conclusion":"SUCCESS","status":"COMPLETED"},
      {"__typename":"CheckRun","conclusion":"","status":"IN_PROGRESS"},
      {"__typename":"CheckRun","conclusion":"FAILURE","status":"COMPLETED"}
   ]}}}}]}},
  {"__typename":"Issue","number":7,"title":"an issue",
   "url":"https://github.com/kukv/octoscope/issues/7",
   "updatedAt":"2026-09-05T12:00:00Z","author":{"login":"kukv"},
   "repository":{"nameWithOwner":"kukv/octoscope"}}
]}}}`

func TestASearchResultBecomesWorkItems(t *testing.T) {
	t.Parallel()

	f := &fake{body: []byte(workJSON)}
	c := f.client()

	items, err := c.ListWorkSection(context.Background(), gh.SectionReviewRequested)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	if !slices.Contains(f.vars[0], S("search", "is:open is:pr review-requested:@me")) {
		t.Errorf("vars %v carry no search variable for the section's own query", f.vars[0])
	}
	if len(items) != 2 {
		t.Fatalf("the column holds %d items, want 2", len(items))
	}

	item := items[0]
	if item.Ref.Kind != gh.ItemPR {
		t.Errorf("kind: got %v, want ItemPR", item.Ref.Kind)
	}
	if item.Ref.Repo != "kukv/octoscope" {
		t.Errorf("repo: got %q, want kukv/octoscope", item.Ref.Repo)
	}
	if item.Review != gh.ReviewRequired {
		t.Errorf("review: got %v, want ReviewRequired", item.Review)
	}
	if item.Checks.Total != 3 || item.Checks.Passed != 1 ||
		item.Checks.Failed != 1 || item.Checks.Running != 1 {
		t.Errorf("checks counts: got %+v", item.Checks)
	}
	if item.Checks.State != gh.CheckFailure {
		t.Errorf("checks state: got %v, want CheckFailure", item.Checks.State)
	}

	if items[1].Ref.Kind != gh.ItemIssue {
		t.Errorf("second item: got %v, want ItemIssue", items[1].Ref.Kind)
	}
}

func TestListWorkSectionReportsAFailure(t *testing.T) {
	t.Parallel()

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) { return []byte("not json"), nil }}

	if _, err := c.ListWorkSection(context.Background(), gh.SectionAssigned); err == nil {
		t.Error("ListWorkSection accepted a body that is not JSON")
	}
}

func TestChecksNoCommitsYieldsCheckNone(t *testing.T) {
	t.Parallel()

	const noRollupJSON = `{"data":{"results":{"nodes":[
	  {"__typename":"PullRequest","number":1,"title":"no checks yet",
	   "url":"https://github.com/kukv/octoscope/pull/1","isDraft":false,
	   "updatedAt":"2026-09-06T12:00:00Z","reviewDecision":"",
	   "author":{"login":"someone"},
	   "repository":{"nameWithOwner":"kukv/octoscope"},
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}}
	]}}}`

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) { return []byte(noRollupJSON), nil }}

	items, err := c.ListWorkSection(context.Background(), gh.SectionReviewRequested)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("the column holds %d items, want 1", len(items))
	}
	if got := items[0].Checks; !reflect.DeepEqual(got, gh.Checks{State: gh.CheckNone}) {
		t.Errorf("checks = %+v, want zero counts with CheckNone", got)
	}
}

func TestCheckOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node CheckContext
		want gh.CheckState
	}{
		{"CheckRun success", CheckContext{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"}, gh.CheckSuccess},
		{"CheckRun failure", CheckContext{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"}, gh.CheckFailure},
		{"CheckRun in progress", CheckContext{Typename: "CheckRun", Status: "IN_PROGRESS"}, gh.CheckRunning},
		{"StatusContext success", CheckContext{Typename: "StatusContext", State: "SUCCESS"}, gh.CheckSuccess},
		{"StatusContext failure", CheckContext{Typename: "StatusContext", State: "FAILURE"}, gh.CheckFailure},
		{"StatusContext error", CheckContext{Typename: "StatusContext", State: "ERROR"}, gh.CheckFailure},
		{"StatusContext pending", CheckContext{Typename: "StatusContext", State: "PENDING"}, gh.CheckPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := checkOutcome(tt.node); got != tt.want {
				t.Errorf("checkOutcome(%+v) = %v, want %v", tt.node, got, tt.want)
			}
		})
	}
}

// TestEachCheckKeepsItsOwnName covers the field the two rollup shapes spell
// differently: a CheckRun calls its name "name", a StatusContext calls it
// "context". Reading only one of them leaves half the drawer's list blank.
func TestEachCheckKeepsItsOwnName(t *testing.T) {
	t.Parallel()

	const namedJSON = `{"data":{"results":{"nodes":[
	  {"__typename":"PullRequest","number":1,"title":"named checks",
	   "url":"https://github.com/kukv/octoscope/pull/1","isDraft":false,
	   "bodyText":"the body","updatedAt":"2026-09-06T12:00:00Z","reviewDecision":"",
	   "author":{"login":"someone"},
	   "repository":{"nameWithOwner":"kukv/octoscope"},
	   "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
	      {"__typename":"CheckRun","name":"build","conclusion":"SUCCESS","status":"COMPLETED"},
	      {"__typename":"StatusContext","context":"ci/legacy","state":"FAILURE"}
	   ]}}}}]}}
	]}}}`

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) { return []byte(namedJSON), nil }}

	items, err := c.ListWorkSection(context.Background(), gh.SectionReviewRequested)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	item := items[0]
	want := []gh.CheckRun{
		{Name: "build", State: gh.CheckSuccess, Kind: gh.CheckKindRun},
		{Name: "ci/legacy", State: gh.CheckFailure, Kind: gh.CheckKindStatus},
	}
	if !reflect.DeepEqual(item.Checks.Runs, want) {
		t.Errorf("runs = %+v, want %+v", item.Checks.Runs, want)
	}
	if item.Body != "the body" {
		t.Errorf("body = %q, want the body the drawer shows", item.Body)
	}
}

// querySelections is the query document with its comments stripped. A field
// name mentioned only in prose must not count as selected.
var querySelections = regexp.MustCompile(`(?m)#.*$`).ReplaceAllString(workQuery, "")

// asksFor reports whether the query document selects the field name, as a
// whole word: "status" must not be satisfied by "statusCheckRollup".
func asksFor(name string) bool {
	return regexp.MustCompile(`(^|\W)` + regexp.QuoteMeta(name) + `(\W|$)`).MatchString(querySelections)
}

// jsonNames collects the JSON field names of t and of every struct reachable
// through it.
func jsonNames(t reflect.Type) []string {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	var names []string
	for i := range t.NumField() {
		f := t.Field(i)
		if tag := f.Tag.Get("json"); tag != "" {
			names = append(names, tag)
		}
		names = append(names, jsonNames(f.Type)...)
	}
	return names
}

// TestTheQueryAsksForEveryFieldWeParse ties work.graphql to the structs
// ListWorkSection unmarshals into. A field the query stops selecting still parses,
// as a zero value that looks like real data -- an empty title, a draft that
// is never a draft -- and no other test notices.
func TestTheQueryAsksForEveryFieldWeParse(t *testing.T) {
	t.Parallel()

	names := jsonNames(reflect.TypeOf(searchNode{}))
	names = append(names, jsonNames(reflect.TypeOf(CheckContext{}))...)
	if len(names) < 15 {
		t.Fatalf("walked only %d fields (%v); the walk is not reaching the nested structs", len(names), names)
	}
	for _, name := range names {
		if !asksFor(name) {
			t.Errorf("the query never selects %q, so it always parses as a zero value", name)
		}
	}
}

// TestTheQueryCarriesTheAliasWeReadBack guards the other seam:
// ListWorkSection reads the column out of "results", and a document that
// aliased the search differently would read back empty rather than failing.
func TestTheQueryCarriesTheAliasWeReadBack(t *testing.T) {
	t.Parallel()

	if !strings.Contains(workQuery, "results: search(") {
		t.Errorf("the query has no search aliased %q:\n%s", "results", workQuery)
	}
}

// GitHub caps a labels connection's first at 100; asking for less silently
// drops labels past that count.
func TestWorkQueryAsksForAsManyLabelsAsGitHubAllows(t *testing.T) {
	t.Parallel()

	if strings.Contains(workQuery, "labels(first: 10)") {
		t.Error("work.graphql still asks for 10 labels; GitHub allows 100")
	}
	if n := strings.Count(workQuery, "labels(first: 100)"); n != 2 {
		t.Errorf("labels(first: 100) appears %d times, want 2 (PullRequest and Issue)", n)
	}
}

// 101 is refused outright with EXCESSIVE_PAGINATION, failing the whole
// document.
func TestNoConnectionAsksForMoreThanGitHubAllows(t *testing.T) {
	t.Parallel()

	docs := map[string]string{
		"work.graphql":   workQuery,
		"checks.graphql": checksQuery,
		"review.graphql": reviewContextQuery,
	}
	re := regexp.MustCompile(`first:\s*(\d+)`)
	for name, doc := range docs {
		for _, m := range re.FindAllStringSubmatch(doc, -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if n > 100 {
				t.Errorf("%s: %s exceeds GitHub's cap of 100", name, m[0])
			}
		}
	}
}

// A recording is the only way to know the alias the query declares still
// matches the key the answer carries.
func TestListWorkSectionParsesARecordedResponse(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/work_section.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var doc struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal recording: %v", err)
	}
	gotAliases := make([]string, 0, len(doc.Data))
	for k := range doc.Data {
		gotAliases = append(gotAliases, k)
	}
	slices.Sort(gotAliases)
	if !slices.Equal(gotAliases, []string{"results"}) {
		t.Errorf("recorded aliases = %v, want [results]", gotAliases)
	}

	c := fileClient(t, "testdata/work_section.json")
	items, err := c.ListWorkSection(context.Background(), gh.SectionAssigned)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no work items parsed out of the recording")
	}
	kinds := map[gh.ItemKind]bool{}
	for _, item := range items {
		kinds[item.Ref.Kind] = true
		if item.Ref.Repo == "" {
			t.Errorf("%q has no repo; the card cannot be opened", item.Title)
		}
		if item.Ref.Number == 0 {
			t.Errorf("%q has no number", item.Title)
		}
		if item.URL == "" {
			t.Errorf("%q has no url", item.Title)
		}
		// The recording is is:open assignee:@me, so anything else means the
		// fixture predates the state field and was not re-recorded.
		if item.State != gh.StateOpen {
			t.Errorf("%q came back %v; re-record work_section.json", item.Title, item.State)
		}
	}
	// The assigned column is the one recorded because it mixes the two
	// shapes: a recording of pull requests alone never runs the Issue branch.
	if !kinds[gh.ItemPR] || !kinds[gh.ItemIssue] {
		t.Errorf("the recording holds only %v; it must exercise both branches", kinds)
	}
}

// The Work board is always is:open, but the same document answers the Search
// tab, whose results carry closed and merged items.
func TestTheSearchDocumentSelectsTheState(t *testing.T) {
	t.Parallel()

	prBlock, issueBlock := onTypeBlocks(t, workQuery)
	if !strings.Contains(prBlock, "state") {
		t.Errorf("the PullRequest selection does not ask for state:\n%s", prBlock)
	}
	if !strings.Contains(issueBlock, "state") {
		t.Errorf("the Issue selection does not ask for state:\n%s", issueBlock)
	}
}

func TestAMergedPullRequestComesBackMerged(t *testing.T) {
	t.Parallel()

	const merged = `{"data":{"results":{"nodes":[
	  {"__typename":"PullRequest","number":9,"title":"merged one","state":"MERGED",
	   "url":"https://github.com/kukv/octoscope/pull/9",
	   "updatedAt":"2026-09-06T12:00:00Z","author":{"login":"kukv"},
	   "repository":{"nameWithOwner":"kukv/octoscope"}}
	]}}}`

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) { return []byte(merged), nil }}
	items, err := c.ListWorkSection(context.Background(), gh.SectionAssigned)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0].State != gh.StateMerged {
		t.Errorf("State = %v, want StateMerged", items[0].State)
	}
}

// onTypeBlocks cuts the document into what it selects for a pull request and
// what it selects for an issue. Searching the whole document for "state"
// would pass while only one of the two branches asked for it.
func onTypeBlocks(t *testing.T, doc string) (pr, issue string) {
	t.Helper()

	prAt := strings.Index(doc, "... on PullRequest")
	issueAt := strings.Index(doc, "... on Issue")
	if prAt < 0 || issueAt < 0 || prAt > issueAt {
		t.Fatalf("the document does not hold a PullRequest block before an Issue block:\n%s", doc)
	}
	// The fragments below the query select a field called "state" of their
	// own (StatusContext). An Issue block that ran to the end of the
	// document would find it and pass whatever the Issue itself selects.
	end := strings.Index(doc[issueAt:], "\nfragment ")
	if end < 0 {
		t.Fatalf("the document has no fragment after the Issue block:\n%s", doc)
	}
	return doc[prAt:issueAt], doc[issueAt : issueAt+end]
}

// The recording exercises a user's own query, not the fixed board searches.
func TestSearchItemsParsesARecordedSearch(t *testing.T) {
	t.Parallel()

	c := fileClient(t, "testdata/search_items.json")
	items, err := c.SearchItems(context.Background(), "repo:kukv/octoscope")
	if err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("no items parsed out of the recording")
	}
	states := map[gh.ItemState]bool{}
	for _, item := range items {
		states[item.State] = true
		if item.Ref.Repo == "" {
			t.Errorf("%q has no repo; the row cannot be opened", item.Title)
		}
	}
	// The recording was taken with no state qualifier, so it holds more than
	// open ones. A recording that lost that would stop testing the state.
	if len(states) < 2 {
		t.Errorf("the recording holds only %v; re-record it over open and closed items", states)
	}
}
