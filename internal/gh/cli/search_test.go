package cli

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The suggestion row is built from three fields: a name to add, the stars
// that tell two similarly-named repositories apart, and whether it is
// private. Dropping any of them leaves a row the user cannot choose between.
func TestSearchReposAsksForTheFieldsTheDialogDraws(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.SearchRepos(context.Background(), "lipgloss", 5); err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	joined := strings.Join(got, " ")
	for _, field := range []string{"fullName", "stargazersCount", "isPrivate"} {
		if !strings.Contains(joined, field) {
			t.Errorf("args %v ask for no %s", got, field)
		}
	}
}

// What the user typed must reach gh as a search term and never be read as a
// flag. Measured 2026-09-11: with the "--" separator, a query of -lipgloss
// returns charmbracelet/lipgloss instead of being rejected.
func TestSearchReposPassesTheQueryAsATermNotAFlag(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	var got []string
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	const query = "--limit=999"
	if _, err := c.SearchRepos(context.Background(), query, 5); err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	at := -1
	for i, arg := range got {
		if arg == query {
			at = i
		}
	}
	if at < 1 {
		t.Fatalf("the query never reached gh: %v", got)
	}
	if got[at-1] != "--" {
		t.Errorf("args %v put the query where gh would read it as a flag", got)
	}
}

// The recording is what gh actually prints, so a change in its shape is
// caught here rather than by a hand-written body that agrees with the parser.
func TestSearchReposParsesWhatGitHubReturns(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "")
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return os.ReadFile("testdata/search_repos.json")
	}
	got, err := c.SearchRepos(context.Background(), "lipgloss", 5)
	if err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("no candidates parsed from the recording")
	}
	if got[0].Name == "" {
		t.Errorf("first candidate has no name: %+v", got[0])
	}
	var withStars int
	for _, cand := range got {
		if cand.Stars > 0 {
			withStars++
		}
	}
	if withStars == 0 {
		t.Errorf("no candidate carried a star count: %+v", got)
	}
}
