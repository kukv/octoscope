package cli

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/github"
)

const emptyColumnJSON = `{"data":{"results":{"nodes":[]}}}`

// One request per search. Four searches in one request is what made
// GitHub's front end stop answering.
func TestSearchItemsSendsOneSearch(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(emptyColumnJSON), nil
	}

	if _, err := c.SearchItems(context.Background(), "is:open assignee:@me"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}

	if len(got) < 2 || got[0] != "api" || got[1] != "graphql" {
		t.Fatalf("got args %v, want them to start with api graphql", got)
	}
	joined := strings.Join(got, " ")
	if n := strings.Count(joined, "search("); n != 1 {
		t.Errorf("the document holds %d searches, want 1:\n%s", n, joined)
	}
	if !strings.Contains(joined, "assignee:@me") {
		t.Errorf("the query is missing:\n%s", joined)
	}
	if strings.Contains(joined, "review-requested:@me") {
		t.Errorf("another query's search string came along:\n%s", joined)
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
	if errors.Is(err, github.ErrUnauthenticated) || errors.Is(err, github.ErrNotInstalled) {
		t.Errorf("err = %v, want it not to be fatal", err)
	}
}

// The board is where the 502s were being seen: four searches leave GitHub's
// front end four chances to refuse, and a search that gives up on the first
// refusal is the failure this retry exists for.
func TestASearchIsAskedAgainAfterATransientFailure(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	calls := 0
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, github.Classify(github.ErrTransient, "gh api: gh: HTTP 502")
		}
		return []byte(emptyColumnJSON), nil
	}

	if _, err := c.SearchItems(context.Background(), "is:open assignee:@me"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if calls != 2 {
		t.Errorf("gh ran %d times, want 2: the search gave up on a failure worth retrying", calls)
	}
}

const emptyReviewContextJSON = `{"data":{"repository":{"pullRequest":{"id":"PR_1","reviews":{"nodes":[]},"reviewThreads":{"nodes":[]}}}}}`

// TestPRReviewContextInTheWorkingDirectorysRepo is the ordinary case: no
// --repo, so there is no "owner/name" to split and gh has to fill the
// placeholders from the checkout's remote. This guards the wiring between
// Client and gql.Client.RepoVars: without it, PRReviewContext falls back to
// gql.SplitRepoVars, which fails on an empty repo instead of asking gh to
// fill in the placeholders -- breaking review, merge and checks for the
// ordinary case of running octoscope with no --repo.
func TestPRReviewContextInTheWorkingDirectorysRepo(t *testing.T) {
	c := New("/w", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(emptyReviewContextJSON), nil
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
// name to GitHub. The per-call repo is "", so this also guards effectiveRepo
// falling back to the client's own repo ("not-a-repo") before the split.
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

func TestSearchItemsPropagatesRunError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("gh api: no such host")
	c := New("/tmp", "")
	c.run = func(context.Context, string, ...string) ([]byte, error) {
		return nil, wantErr
	}

	items, err := c.SearchItems(context.Background(), "is:open assignee:@me")
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if len(items) != 0 {
		t.Errorf("the result holds %d items, want 0", len(items))
	}
}

// These five calls go through the shared documents now, not through gh's own
// subcommands. A backend that still shells out to `gh pr list` would be
// selecting a second, unchecked copy of the same fields.
func TestTheListingCallsSendAGraphQLDocument(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*Client) error{
		"ListPRs":    func(c *Client) error { _, err := c.ListPRs(context.Background(), "kukv/octoscope"); return err },
		"ListIssues": func(c *Client) error { _, err := c.ListIssues(context.Background(), "kukv/octoscope"); return err },
		"GetPR":      func(c *Client) error { _, err := c.GetPR(context.Background(), "kukv/octoscope", 1); return err },
		"GetIssue":   func(c *Client) error { _, err := c.GetIssue(context.Background(), "kukv/octoscope", 1); return err },
		"RepoName":   func(c *Client) error { _, err := c.RepoName(context.Background()); return err },
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			c := New("/repo", "kukv/octoscope")
			var got []string
			var dir string
			c.run = func(_ context.Context, d string, args ...string) ([]byte, error) {
				dir, got = d, args
				return []byte(`{"data":{}}`), nil
			}
			if err := call(c); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if len(got) < 2 || got[0] != "api" || got[1] != "graphql" {
				t.Errorf("%s ran gh %v, want gh api graphql", name, got)
			}
			// gh resolves {owner} and {repo} from the directory it runs in,
			// so a call that forgets the client's own directory answers for
			// wherever octoscope was started instead.
			if dir != "/repo" {
				t.Errorf("%s ran gh in %q, want /repo", name, dir)
			}
			// The document has to name the repository the client was built
			// for; a call that sends no owner asks GitHub about nothing.
			if !slices.Contains(got, "owner=kukv") || !slices.Contains(got, "name=octoscope") {
				t.Errorf("%s ran gh %v, want it to name kukv/octoscope", name, got)
			}
		})
	}
}

// Inside a checkout with no --repo, gh is the one that knows where we are:
// it fills {owner} and {repo} from the remote. The api backend cannot, which
// is why the two spell the same variables differently.
func TestWithoutARepositoryTheDocumentCarriesGhsPlaceholders(t *testing.T) {
	t.Parallel()

	c := New("", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{}}`), nil
	}
	if _, err := c.ListPRs(context.Background(), ""); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "owner={owner}") || !strings.Contains(joined, "name={repo}") {
		t.Errorf("args = %v, want gh's own placeholders", got)
	}
}
