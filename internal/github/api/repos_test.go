package api

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// The dialog's suggestions are typed into, so a word starting with a dash or
// carrying a space has to survive the trip. A query pasted into the path
// unescaped would be a different search, or a 404.
func TestASearchEscapesTheTypedQueryIntoTheParameter(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "search_repos.json"))
	})
	found, err := c.SearchRepos(context.Background(), "go tui/term", 5)
	if err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no candidates parsed")
	}
	if found[0].Name == "" {
		t.Error("candidate has no name")
	}
	req := (*got)[0]
	if q := req.URL.Query().Get("q"); q != "go tui/term" {
		t.Errorf("q = %q, want the query as typed", q)
	}
	if per := req.URL.Query().Get("per_page"); per != "5" {
		t.Errorf("per_page = %q, want the limit the caller asked for", per)
	}
}

// REST caps a page at 100 and answers 422 above it. The seeding path asks for
// 100, so one more would turn every first run into an error.
func TestASearchNeverAsksForMoreThanAPage(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "search_repos.json"))
	})
	if _, err := c.SearchRepos(context.Background(), "go", 500); err != nil {
		t.Fatalf("SearchRepos: %v", err)
	}
	if per := (*got)[0].URL.Query().Get("per_page"); per != "100" {
		t.Errorf("per_page = %q, want 100", per)
	}
}

// gh repo list shows the repositories the user owns, most recently pushed
// first. Leaving the order to REST's default would put the seeding dialog's
// candidates in a different order than gh's, for the same account.
func TestOwnReposAreTheOnesOwnedMostRecentlyPushedFirst(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "user_repos.json"))
	})
	repos, err := c.ListOwnRepos(context.Background(), "", 100)
	if err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if len(repos) == 0 {
		t.Fatal("no repositories parsed")
	}
	q := (*got)[0].URL.Query()
	if (*got)[0].URL.Path != "/user/repos" {
		t.Errorf("path = %q, want /user/repos for the authenticated user", (*got)[0].URL.Path)
	}
	if q.Get("affiliation") != "owner" {
		t.Errorf("affiliation = %q, want owner", q.Get("affiliation"))
	}
	if q.Get("sort") != "pushed" || q.Get("direction") != "desc" {
		t.Errorf("sort = %q %q, want pushed desc", q.Get("sort"), q.Get("direction"))
	}
}

// A named owner is an organisation -- the seeding path only ever passes one
// of ListOrgs' answers. /users/{owner}/repos would answer too, but shows only
// what is public, hiding exactly the repositories a member joined for.
func TestANamedOwnerReadsTheOrganisationsRepositories(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "user_repos.json"))
	})
	if _, err := c.ListOwnRepos(context.Background(), "kukv", 100); err != nil {
		t.Fatalf("ListOwnRepos: %v", err)
	}
	if (*got)[0].URL.Path != "/orgs/kukv/repos" {
		t.Errorf("path = %q, want /orgs/kukv/repos", (*got)[0].URL.Path)
	}
}

func TestOrgsComeBackAsLogins(t *testing.T) {
	t.Parallel()

	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "user_orgs.json"))
	})
	orgs, err := c.ListOrgs(context.Background())
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	for i, o := range orgs {
		if o == "" {
			t.Errorf("org %d has an empty login", i)
		}
	}
}
