package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// serveREST stands in for GitHub's REST API. It records every request, since
// a call that pages sends more than one.
func serveREST(t *testing.T, handler http.HandlerFunc) (*Client, *[]*http.Request) {
	t.Helper()

	var got []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		clone := r.Clone(r.Context())
		clone.Body = io.NopCloser(strings.NewReader(string(body)))
		got = append(got, clone)
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL
	return c, &got
}

// GitHub refuses an unnamed client and serves a different shape to a client
// that does not pin the API version, so every REST call carries both, plus
// the token this backend exists for.
func TestARestCallNamesItselfAndTheApiVersionAndTheToken(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if _, err := c.read(context.Background(), "repos/kukv/octoscope/labels", ""); err != nil {
		t.Fatalf("read: %v", err)
	}

	req := (*got)[0]
	if req.URL.Path != "/repos/kukv/octoscope/labels" {
		t.Errorf("path = %q, want /repos/kukv/octoscope/labels", req.URL.Path)
	}
	if h := req.Header.Get("Authorization"); h != "bearer secret-token" {
		t.Errorf("Authorization = %q", h)
	}
	if req.Header.Get("User-Agent") == "" {
		t.Error("User-Agent is empty; GitHub may refuse an unnamed client")
	}
	if v := req.Header.Get("X-GitHub-Api-Version"); v == "" {
		t.Error("X-GitHub-Api-Version is empty")
	}
	if a := req.Header.Get("Accept"); a != "application/vnd.github+json" {
		t.Errorf("Accept = %q, want the JSON media type by default", a)
	}
}

// A read is safe to repeat: a 502 says no answer came back, not that nothing
// arrived. gql.Client.Read does the same for GraphQL.
func TestAReadAsksAgainOnceWhenGitHubsFrontEndDidNotAnswer(t *testing.T) {
	t.Parallel()

	calls := 0
	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = io.WriteString(w, `[]`)
	})
	if _, err := c.read(context.Background(), "repos/kukv/octoscope/labels", ""); err != nil {
		t.Fatalf("read: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2", calls)
	}
}

// A write is not safe to repeat: a comment posted twice is two comments.
func TestAWriteIsNotRepeated(t *testing.T) {
	t.Parallel()

	calls := 0
	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	})
	err := func() error {
		_, err := c.write(context.Background(), http.MethodPost, "repos/kukv/octoscope/issues/1/comments", map[string]string{"body": "hi"})
		return err
	}()
	if !errors.Is(err, gh.ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
}

// The files API's default page is 30 entries and one pull request under
// review had 418, so a caller that stops at the first page loses most of the
// diff. The link to the next page is the only thing that says there is more.
func TestNextLinkFindsTheNextPageAmongTheOtherRelations(t *testing.T) {
	t.Parallel()

	h := http.Header{}
	h.Set("Link", `<https://api.github.com/x?page=2>; rel="next", <https://api.github.com/x?page=5>; rel="last"`)
	if got := nextLink(h); got != "https://api.github.com/x?page=2" {
		t.Errorf("nextLink = %q", got)
	}
	if got := nextLink(http.Header{}); got != "" {
		t.Errorf("nextLink with no header = %q, want empty", got)
	}
	last := http.Header{}
	last.Set("Link", `<https://api.github.com/x?page=1>; rel="prev"`)
	if got := nextLink(last); got != "" {
		t.Errorf("nextLink on the last page = %q, want empty", got)
	}
}

// Every REST path needs an owner and a name. An explicit repository wins, then
// the client's own, then the working directory's remote -- the same order
// repoVars uses for GraphQL, so the two halves of this backend cannot disagree
// about which repository a screen is showing.
func TestRepoPathPrefersTheExplicitRepositoryOverTheClients(t *testing.T) {
	t.Parallel()

	c := New("", "kukv/octoscope", "t")
	got, err := c.repoPath("cli/cli")
	if err != nil {
		t.Fatalf("repoPath: %v", err)
	}
	if got != "cli/cli" {
		t.Errorf("repoPath = %q, want cli/cli", got)
	}
}

// send attaches this client's own token to whatever URL it is given, so a
// next link pointing somewhere else must not be followed.
func TestWalkPagesStopsAtANextLinkPointingAtAnotherHost(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Link", `<https://evil.example/x?page=2>; rel="next"`)
		_, _ = io.WriteString(w, `[1]`)
	})
	var pages int
	err := c.walkPages(context.Background(), "x", func([]byte) error {
		pages++
		return nil
	})
	if err != nil {
		t.Fatalf("walkPages: %v", err)
	}
	if pages != 1 {
		t.Errorf("pages = %d, want 1 (the off-host next link must not be followed)", pages)
	}
	if len(*got) != 1 {
		t.Errorf("requests = %d, want 1", len(*got))
	}
}

// A next link pointing back at the same page would loop forever without a
// page cap.
func TestWalkPagesStopsAtMaxPagesWhenNextKeepsPointingAtItself(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Link", `<http://`+r.Host+r.URL.Path+`>; rel="next"`)
		_, _ = io.WriteString(w, `[1]`)
	})
	var pages int
	err := c.walkPages(context.Background(), "x", func([]byte) error {
		pages++
		return nil
	})
	if err != nil {
		t.Fatalf("walkPages: %v", err)
	}
	if pages != maxPages {
		t.Errorf("pages = %d, want %d", pages, maxPages)
	}
	if len(*got) != maxPages {
		t.Errorf("requests = %d, want %d", len(*got), maxPages)
	}
}

// A transient failure on a later page must be retried the same way the first
// page is: prFiles and ListLabels build on walkPages precisely so paging
// callers get this for free.
func TestWalkPagesRetriesATransientFailureOnALaterPage(t *testing.T) {
	t.Parallel()

	calls := 0
	c, _ := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch calls {
		case 1:
			w.Header().Set("Link", `<http://`+r.Host+`/x?page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[1]`)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = io.WriteString(w, `[1]`)
		}
	})
	var pages int
	err := c.walkPages(context.Background(), "x", func([]byte) error {
		pages++
		return nil
	})
	if err != nil {
		t.Fatalf("walkPages: %v", err)
	}
	if pages != 2 {
		t.Errorf("pages = %d, want 2", pages)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3 (page 1, page 2's 502, page 2's retry)", calls)
	}
}

// A repository with no owner/name separator would build a path GitHub reads as
// a different endpoint entirely, so it is refused before the request is sent.
func TestRepoPathRefusesAThingThatIsNotOwnerSlashName(t *testing.T) {
	t.Parallel()

	c := New("", "octoscope", "t")
	if _, err := c.repoPath(""); err == nil {
		t.Fatal("repoPath accepted a repository with no owner")
	}
}
