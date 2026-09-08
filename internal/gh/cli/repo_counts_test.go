package cli

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// The document must not carry a repository name: the names come from the
// settings file, and only GraphQL variables keep them out of the query text.
func TestRepoCountsPassesTheNamesAsVariables(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return os.ReadFile("testdata/repo_counts.json")
	}

	if _, err := c.RepoCounts(context.Background(), []string{"kukv/octoscope", "cli/cli"}); err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	query := varValue(got, "query")
	for _, name := range []string{"kukv", "octoscope", "cli"} {
		if strings.Contains(query, name) {
			t.Errorf("the query text carries %q; it must travel as a variable", name)
		}
	}
	if varValue(got, "o0") != "kukv" || varValue(got, "n0") != "octoscope" {
		t.Errorf("first repository was not passed as variables: %v", got)
	}
}

// The badge is per repository, so the counts have to come back split by
// repository rather than summed.
func TestRepoCountsReadsEachRepositorysOwnCounts(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return os.ReadFile("testdata/repo_counts.json")
	}

	got, err := c.RepoCounts(context.Background(), []string{"kukv/octoscope", "cli/cli"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d counts, want 2", len(got))
	}
	if got[0].Repo != "kukv/octoscope" || got[1].Repo != "cli/cli" {
		t.Errorf("counts came back in another order: %+v", got)
	}
	if got[0].PRs == got[1].PRs && got[0].Issues == got[1].Issues {
		t.Errorf("both repositories got the same counts: %+v", got)
	}
}

// One repository the user renamed or lost access to must not cost the badges
// of every other row.
func TestRepoCountsKeepsTheAnswersItGotWhenOneRepositoryIsGone(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		raw, err := os.ReadFile("testdata/repo_counts_partial.json")
		if err != nil {
			return nil, err
		}
		return raw, errors.New("gh api: Could not resolve to a Repository")
	}

	got, err := c.RepoCounts(context.Background(), []string{"kukv/octoscope", "kukv/no-such-repository-xyz"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d counts, want 2", len(got))
	}
	if got[0].Unavailable {
		t.Errorf("the repository that answered was marked unavailable: %+v", got[0])
	}
	if !got[1].Unavailable {
		t.Errorf("the repository that could not be resolved was not marked: %+v", got[1])
	}
}

// A response with no "data" at all -- an expired token, a rate limit, a
// validation error -- means the whole document was rejected, not that every
// repository failed to resolve. Reading it the same way as a partial failure
// would tell a user whose token expired that every repository disappeared.
//
// This body cannot be recorded without actually expiring a token or hitting
// a rate limit, so it is written by hand; the shape ("errors" with no
// "data") is what gh api graphql's own documentation describes for a
// request-level failure.
func TestRepoCountsFailsWhenTheWholeQueryIsRejected(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	wantErr := errors.New("gh api: Bad credentials")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte(`{"errors":[{"message":"Bad credentials"}]}`), wantErr
	}

	got, err := c.RepoCounts(context.Background(), []string{"kukv/octoscope", "cli/cli"})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Errorf("got %+v, want no counts when the request itself failed", got)
	}
}

// The alias a repository is sent under is a separate count from its
// position in the input: a malformed name earlier in the list is not sent
// at all, so the first repository actually sent becomes alias r0 while
// landing at index 1 in the result.
func TestRepoCountsMatchesAliasesBackToTheRightIndexWhenAnEarlierNameIsSkipped(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return os.ReadFile("testdata/repo_counts.json")
	}

	got, err := c.RepoCounts(context.Background(), []string{"not-a-repository", "kukv/octoscope"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d counts, want 2", len(got))
	}
	if !got[0].Unavailable {
		t.Errorf("got[0] = %+v, want the malformed name marked unavailable", got[0])
	}
	if got[1].Unavailable || got[1].PRs != 0 || got[1].Issues != 2 {
		t.Errorf("got[1] = %+v, want r0's counts (PRs 0, Issues 2) from the recording", got[1])
	}
}

// A line the user typed by hand can be anything; it must not become a
// request.
func TestRepoCountsDoesNotAskAboutAMalformedName(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	called := false
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		called = true
		return []byte(`{"data":{}}`), nil
	}

	got, err := c.RepoCounts(context.Background(), []string{"not-a-repository"})
	if err != nil {
		t.Fatalf("RepoCounts: %v", err)
	}
	if called {
		t.Error("a malformed name was sent to GitHub")
	}
	if len(got) != 1 || !got[0].Unavailable {
		t.Errorf("got %+v, want one unavailable row", got)
	}
}

// varValue returns the value of the GraphQL variable name, passed as
// "-f name=value" or "-F name=value".
func varValue(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] != "-f" && args[i] != "-F" {
			continue
		}
		if k, v, ok := strings.Cut(args[i+1], "="); ok && k == name {
			return v
		}
	}
	return ""
}
