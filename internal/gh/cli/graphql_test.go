package cli

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

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

func TestSearchItemsSendsTheQueryItWasGiven(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(emptyColumnJSON), nil
	}

	if _, err := c.SearchItems(context.Background(), "is:pr org:kukv label:renovate"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}

	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "search=is:pr org:kukv label:renovate") {
		t.Errorf("the query never reached gh:\n%s", joined)
	}
	// The query travels as a variable. A query that became part of the
	// document could not hold a quote or a brace.
	if strings.Contains(joined, `search(type: ISSUE, first: 50, query: "is:pr`) {
		t.Errorf("the query was pasted into the document:\n%s", joined)
	}
}

// The document travels as gh's own "query" parameter, so the search string
// has to go under a different name or it would overwrite the document.
func TestTheSearchStringDoesNotTravelAsQuery(t *testing.T) {
	t.Parallel()

	c := New("", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"results":{"nodes":[]}}}`), nil
	}
	if _, err := c.SearchItems(context.Background(), "is:open"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if !slices.Contains(got, "search=is:open") {
		t.Errorf("args %q carry no search variable", got)
	}
	for _, a := range got {
		if a == "query=is:open" {
			t.Error("the search string was sent as the document")
		}
	}
}

// A query the user typed can be one GitHub rejects. What it said is the only
// thing that tells them how to fix it, so it must not be swallowed.
func TestSearchItemsReportsWhatGitHubSaidAboutABadQuery(t *testing.T) {
	t.Parallel()

	const rejected = `{"data":null,"errors":[{"message":"Invalid search query"}]}`
	c, _ := newTestClient(rejected, errors.New(`gh api: {"message":"Invalid search query"}`))

	_, err := c.SearchItems(t.Context(), "is:nonsense")
	if err == nil {
		t.Fatal("SearchItems succeeded on a query GitHub rejected")
	}
	if !strings.Contains(err.Error(), "Invalid search query") {
		t.Errorf("err = %v, want it to carry what GitHub said", err)
	}
	// The Search tab shows this on its notice line. A fatal error would
	// replace the whole screen over one mistyped query.
	if gh.IsFatal(err) {
		t.Errorf("err = %v, want it not to be fatal", err)
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
