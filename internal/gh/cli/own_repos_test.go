package cli

import (
	"context"
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// gh repo list fetches 30 by default and says nothing about the rest, so an
// account with more repositories than that would lose them from the list a
// first run is seeded from.
func TestListOwnReposAsksForMoreThanTheDefaultThirty(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.ListOwnRepos(context.Background(), "", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	limit, ok := flagValue(got, "--limit")
	if !ok {
		t.Fatalf("args %v carry no --limit", got)
	}
	n, err := strconv.Atoi(limit)
	if err != nil || n <= 30 {
		t.Errorf("--limit = %q, want a number above gh's default of 30", limit)
	}
}

// "My repositories" and "the organisation's" are the same subcommand: gh
// repo list takes the owner as a positional argument and lists the
// authenticated user when given none.
func TestListOwnReposNamesTheOwnerOnlyWhenGivenOne(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.ListOwnRepos(context.Background(), "charmbracelet", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if !slices.Contains(got, "charmbracelet") {
		t.Errorf("args %v never name the owner", got)
	}
	if _, err := c.ListOwnRepos(context.Background(), "", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if slices.Contains(got, "charmbracelet") {
		t.Errorf("the ownerless call carried an owner: %v", got)
	}
}

// The recording is what gh actually prints for repo list, which names its
// field differently from the search subcommand.
func TestListOwnReposParsesWhatGitHubReturns(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return os.ReadFile("testdata/own_repos.json")
	}
	got, err := c.ListOwnRepos(context.Background(), "", 100)
	if err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no repositories parsed from the recording")
	}
	for _, r := range got {
		if _, _, ok := gh.SplitRepo(r.Name); !ok {
			t.Errorf("%q is not owner/name", r.Name)
		}
	}
}

// The response to user/orgs carries a member's whole view of an
// organisation. Narrowing it in gh keeps everything but the logins from ever
// being decoded here.
func TestListOrgsReadsTheLoginsOnly(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[\"charmbracelet\"]\n"), nil
	}
	orgs, err := c.ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	if !slices.Equal(orgs, []string{"charmbracelet"}) {
		t.Errorf("orgs = %v, want [charmbracelet]", orgs)
	}
	jq, ok := flagValue(got, "--jq")
	if !ok {
		t.Fatalf("args %v do not narrow the response", got)
	}
	if !slices.Contains(got, "user/orgs") {
		t.Errorf("args %v ask for something other than the user's organisations", got)
	}
	if jq != "[.[].login]" {
		t.Errorf("--jq = %q, want the logins as a JSON array", jq)
	}
}

// flagValue returns the argument that follows name.
func flagValue(args []string, name string) (string, bool) {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}
