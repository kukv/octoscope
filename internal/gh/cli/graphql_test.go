package cli

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

const workJSON = `{"data":{
  "reviewRequested":{"nodes":[
    {"__typename":"PullRequest","number":12,"title":"fix the thing",
     "url":"https://github.com/kukv/octoscope/pull/12","isDraft":false,
     "updatedAt":"2026-09-06T12:00:00Z","reviewDecision":"REVIEW_REQUIRED",
     "author":{"login":"someone"},
     "repository":{"nameWithOwner":"kukv/octoscope"},
     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
        {"__typename":"CheckRun","conclusion":"SUCCESS","status":"COMPLETED"},
        {"__typename":"CheckRun","conclusion":"","status":"IN_PROGRESS"},
        {"__typename":"CheckRun","conclusion":"FAILURE","status":"COMPLETED"}
     ]}}}}]}}
  ]},
  "yourPRs":{"nodes":[]},
  "assigned":{"nodes":[
    {"__typename":"Issue","number":7,"title":"an issue",
     "url":"https://github.com/kukv/octoscope/issues/7",
     "updatedAt":"2026-09-05T12:00:00Z","author":{"login":"kukv"},
     "repository":{"nameWithOwner":"kukv/octoscope"}}
  ]},
  "mentioned":{"nodes":[]}
}}`

func TestListWorkBuildsOneGraphQLRequest(t *testing.T) {
	t.Parallel()

	var got []string
	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(workJSON), nil
	}

	if _, err := c.ListWork(context.Background()); err != nil {
		t.Fatalf("ListWork: %v", err)
	}

	if len(got) < 2 || got[0] != "api" || got[1] != "graphql" {
		t.Fatalf("got args %v, want them to start with api graphql", got)
	}
	query := flagValue(t, got, "-f")
	for _, want := range []string{
		"review-requested:@me", "author:@me", "assignee:@me", "mentions:@me",
		"reviewDecision", "statusCheckRollup",
	} {
		if !strings.Contains(query, want) {
			t.Errorf("query is missing %q:\n%s", want, query)
		}
	}
}

func TestListWorkTranslatesToDomainValues(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(workJSON), nil
	}

	w, err := c.ListWork(context.Background())
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}

	rr := w[gh.SectionReviewRequested]
	if len(rr) != 1 {
		t.Fatalf("review requested holds %d items, want 1", len(rr))
	}
	item := rr[0]
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

	assigned := w[gh.SectionAssigned]
	if len(assigned) != 1 || assigned[0].Ref.Kind != gh.ItemIssue {
		t.Fatalf("assigned column: got %+v, want one issue", assigned)
	}
	if n := len(w[gh.SectionYourPRs]); n != 0 {
		t.Errorf("your PRs holds %d items, want 0", n)
	}
}

func TestListWorkReportsAFailure(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("not json"), nil
	}

	if _, err := c.ListWork(context.Background()); err == nil {
		t.Error("ListWork accepted a body that is not JSON")
	}
}

func TestListWorkPropagatesRunError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("gh api: HTTP 502")
	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, wantErr
	}

	w, err := c.ListWork(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	for _, s := range gh.WorkSections() {
		if n := len(w[s]); n != 0 {
			t.Errorf("section %v holds %d items, want 0", s, n)
		}
	}
}

func TestChecksNoCommitsYieldsCheckNone(t *testing.T) {
	t.Parallel()

	const noRollupJSON = `{"data":{
	  "reviewRequested":{"nodes":[
	    {"__typename":"PullRequest","number":1,"title":"no checks yet",
	     "url":"https://github.com/kukv/octoscope/pull/1","isDraft":false,
	     "updatedAt":"2026-09-06T12:00:00Z","reviewDecision":"",
	     "author":{"login":"someone"},
	     "repository":{"nameWithOwner":"kukv/octoscope"},
	     "commits":{"nodes":[{"commit":{"statusCheckRollup":null}}]}}
	  ]},
	  "yourPRs":{"nodes":[]},
	  "assigned":{"nodes":[]},
	  "mentioned":{"nodes":[]}
	}}`

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(noRollupJSON), nil
	}

	w, err := c.ListWork(context.Background())
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	rr := w[gh.SectionReviewRequested]
	if len(rr) != 1 {
		t.Fatalf("review requested holds %d items, want 1", len(rr))
	}
	if got := rr[0].Checks; !reflect.DeepEqual(got, gh.Checks{State: gh.CheckNone}) {
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

	const namedJSON = `{"data":{
	  "reviewRequested":{"nodes":[
	    {"__typename":"PullRequest","number":1,"title":"named checks",
	     "url":"https://github.com/kukv/octoscope/pull/1","isDraft":false,
	     "bodyText":"the body","updatedAt":"2026-09-06T12:00:00Z","reviewDecision":"",
	     "author":{"login":"someone"},
	     "repository":{"nameWithOwner":"kukv/octoscope"},
	     "commits":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
	        {"__typename":"CheckRun","name":"build","conclusion":"SUCCESS","status":"COMPLETED"},
	        {"__typename":"StatusContext","context":"ci/legacy","state":"FAILURE"}
	     ]}}}}]}}
	  ]},
	  "yourPRs":{"nodes":[]},
	  "assigned":{"nodes":[]},
	  "mentioned":{"nodes":[]}
	}}`

	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return []byte(namedJSON), nil
	}

	w, err := c.ListWork(context.Background())
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	item := w[gh.SectionReviewRequested][0]
	want := []gh.CheckRun{
		{Name: "build", State: gh.CheckSuccess},
		{Name: "ci/legacy", State: gh.CheckFailure},
	}
	if !reflect.DeepEqual(item.Checks.Runs, want) {
		t.Errorf("runs = %+v, want %+v", item.Checks.Runs, want)
	}
	if item.Body != "the body" {
		t.Errorf("body = %q, want the body the drawer shows", item.Body)
	}
}

// flagValue returns the argument that follows the last occurrence of flag.
func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	for i := len(args) - 2; i >= 0; i-- {
		if args[i] == flag {
			return args[i+1]
		}
	}
	t.Fatalf("flag %q not found in %v", flag, args)
	return ""
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
// ListWork unmarshals into. A field the query stops selecting still parses,
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

// TestTheQueryCarriesEveryAliasWeReadBack guards the other seam: ListWork
// keys the columns by the aliases in workSearches, and a column whose alias
// is absent from the document reads back empty rather than failing.
func TestTheQueryCarriesEveryAliasWeReadBack(t *testing.T) {
	t.Parallel()

	for _, s := range workSearches {
		if !strings.Contains(workQuery, s.alias+": search(") {
			t.Errorf("the query has no search aliased %q", s.alias)
		}
	}
}

// A label connection asked for 10 silently drops the eleventh label, and a
// Work card is where labels are read. GitHub caps first at 100, so there is
// no reason to ask for less.
func TestWorkQueryAsksForAsManyLabelsAsGitHubAllows(t *testing.T) {
	t.Parallel()

	if strings.Contains(workQuery, "labels(first: 10)") {
		t.Error("work.graphql still asks for 10 labels; GitHub allows 100")
	}
	if n := strings.Count(workQuery, "labels(first: 100)"); n != 2 {
		t.Errorf("labels(first: 100) appears %d times, want 2 (PullRequest and Issue)", n)
	}
}

// Every connection GitHub caps at 100 must ask for no more: 101 is refused
// outright with EXCESSIVE_PAGINATION, which fails the whole document.
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

// ListWork reads four aliased searches out of one response. A recording is
// the only way to know the aliases the query declares and the keys the answer
// carries are still the same four.
func TestListWorkParsesARecordedResponse(t *testing.T) {
	c, _ := newTestClient(readTestdata(t, "work.json"), nil)

	w, err := c.ListWork(t.Context())
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	for _, section := range gh.WorkSections() {
		for _, item := range w[section] {
			if item.Ref.Repo == "" {
				t.Errorf("section %d: %q has no repo; the card cannot be opened", section, item.Title)
			}
			if item.Ref.Number == 0 {
				t.Errorf("section %d: %q has no number", section, item.Title)
			}
			if item.URL == "" {
				t.Errorf("section %d: %q has no url", section, item.Title)
			}
		}
	}
}
