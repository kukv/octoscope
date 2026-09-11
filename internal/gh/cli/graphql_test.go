package cli

import (
	"context"
	"encoding/json"
	"errors"
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

const emptyColumnJSON = `{"data":{"results":{"nodes":[]}}}`

// One request per column, each carrying only its own search string. Four
// searches in one request is what made GitHub's front end stop answering.
func TestListWorkSectionSendsOneSearch(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(emptyColumnJSON), nil
	}

	if _, err := c.ListWorkSection(context.Background(), gh.SectionAssigned); err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}

	if len(got) < 2 || got[0] != "api" || got[1] != "graphql" {
		t.Fatalf("got args %v, want them to start with api graphql", got)
	}
	joined := strings.Join(got, " ")
	if n := strings.Count(joined, "search("); n != 1 {
		t.Errorf("the document holds %d searches, want 1:\n%s", n, joined)
	}
	if !strings.Contains(joined, "assignee:@me") {
		t.Errorf("the assigned column's search string is missing:\n%s", joined)
	}
	if strings.Contains(joined, "review-requested:@me") {
		t.Errorf("another column's search string came along:\n%s", joined)
	}
	for _, want := range []string{"reviewDecision", "statusCheckRollup"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the document is missing %q:\n%s", want, joined)
		}
	}
}

// TestEverySectionHasItsOwnSearch pins what each column of the board means.
// The search string is not an implementation detail the code happens to
// build: "review requested" is defined by review-requested:@me and by
// nothing else, and a column paired with the wrong one silently shows the
// wrong work. Asserting only that the four differ leaves two of them free to
// swap.
func TestEverySectionHasItsOwnSearch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		section gh.WorkSection
		search  string
	}{
		{"review requested", gh.SectionReviewRequested, "is:open is:pr review-requested:@me"},
		{"your PRs", gh.SectionYourPRs, "is:open is:pr author:@me"},
		{"assigned", gh.SectionAssigned, "is:open assignee:@me"},
		{"mentioned", gh.SectionMentioned, "is:open mentions:@me"},
	}
	// A column added without a line here would go untested rather than fail.
	if len(tests) != gh.WorkSectionCount {
		t.Fatalf("the table covers %d columns, the board has %d", len(tests), gh.WorkSectionCount)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			c := New("/tmp", "")
			var search string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				for _, a := range args {
					if rest, ok := strings.CutPrefix(a, "search="); ok {
						search = rest
					}
				}
				return []byte(emptyColumnJSON), nil
			}
			if _, err := c.ListWorkSection(context.Background(), tt.section); err != nil {
				t.Fatalf("ListWorkSection: %v", err)
			}
			if search != tt.search {
				t.Errorf("%s sends %q, want %q", tt.name, search, tt.search)
			}
		})
	}
}

// The board is where the 502s were being seen: four searches leave GitHub's
// front end four chances to refuse, and a column that gives up on the first
// refusal is the failure this retry exists for.
func TestAWorkSectionIsAskedAgainAfterATransientFailure(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	calls := 0
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, gh.Classify(gh.ErrTransient, "gh api: gh: HTTP 502")
		}
		return []byte(emptyColumnJSON), nil
	}

	if _, err := c.ListWorkSection(context.Background(), gh.SectionAssigned); err != nil {
		t.Fatalf("ListWorkSection: %v", err)
	}
	if calls != 2 {
		t.Errorf("gh ran %d times, want 2: the column gave up on a failure worth retrying", calls)
	}
}

// A section outside the board is a bug in the caller, not a search GitHub
// should be asked to run.
func TestListWorkSectionRejectsASectionTheBoardDoesNotHave(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		t.Error("an unknown section was sent to gh")
		return []byte(emptyColumnJSON), nil
	}
	if _, err := c.ListWorkSection(context.Background(), gh.WorkSectionCount); err == nil {
		t.Error("ListWorkSection accepted a section the board does not have")
	}
}

func TestListWorkSectionTranslatesToDomainValues(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(workJSON), nil
	}

	items, err := c.ListWorkSection(context.Background(), gh.SectionReviewRequested)
	if err != nil {
		t.Fatalf("ListWorkSection: %v", err)
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

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("not json"), nil
	}

	if _, err := c.ListWorkSection(context.Background(), gh.SectionAssigned); err == nil {
		t.Error("ListWorkSection accepted a body that is not JSON")
	}
}

func TestListWorkSectionPropagatesRunError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("gh api: no such host")
	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, wantErr
	}

	items, err := c.ListWorkSection(context.Background(), gh.SectionAssigned)
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if len(items) != 0 {
		t.Errorf("the column holds %d items, want 0", len(items))
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

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(noRollupJSON), nil
	}

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
		node checkNode
		want gh.CheckState
	}{
		{"CheckRun success", checkNode{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"}, gh.CheckSuccess},
		{"CheckRun failure", checkNode{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"}, gh.CheckFailure},
		{"CheckRun in progress", checkNode{Typename: "CheckRun", Status: "IN_PROGRESS"}, gh.CheckRunning},
		{"StatusContext success", checkNode{Typename: "StatusContext", State: "SUCCESS"}, gh.CheckSuccess},
		{"StatusContext failure", checkNode{Typename: "StatusContext", State: "FAILURE"}, gh.CheckFailure},
		{"StatusContext error", checkNode{Typename: "StatusContext", State: "ERROR"}, gh.CheckFailure},
		{"StatusContext pending", checkNode{Typename: "StatusContext", State: "PENDING"}, gh.CheckPending},
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

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(namedJSON), nil
	}

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
	names = append(names, jsonNames(reflect.TypeOf(checkNode{}))...)
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
	raw := readTestdata(t, "work_section.json")

	var doc struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
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

	c, _ := newTestClient(raw, nil)
	items, err := c.ListWorkSection(t.Context(), gh.SectionAssigned)
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
	}
	// The assigned column is the one recorded because it mixes the two
	// shapes: a recording of pull requests alone never runs the Issue branch.
	if !kinds[gh.ItemPR] || !kinds[gh.ItemIssue] {
		t.Errorf("the recording holds only %v; it must exercise both branches", kinds)
	}
}
