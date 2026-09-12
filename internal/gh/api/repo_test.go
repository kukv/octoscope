package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

func TestParseRemoteReadsEveryShapeGitWritesTheURLIn(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		url  string
		want string
	}{
		"scp-like ssh":      {"git@github.com:kukv/octoscope.git", "kukv/octoscope"},
		"ssh url":           {"ssh://git@github.com/kukv/octoscope.git", "kukv/octoscope"},
		"ssh url no user":   {"ssh://github.com/kukv/octoscope.git", "kukv/octoscope"},
		"https with suffix": {"https://github.com/kukv/octoscope.git", "kukv/octoscope"},
		"https bare":        {"https://github.com/kukv/octoscope", "kukv/octoscope"},
		"https with user":   {"https://kukv@github.com/kukv/octoscope.git", "kukv/octoscope"},
		"trailing slash":    {"https://github.com/kukv/octoscope/", "kukv/octoscope"},
		"trailing newline":  {"https://github.com/kukv/octoscope.git\n", "kukv/octoscope"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := parseRemote(tc.url)
			if err != nil {
				t.Fatalf("parseRemote(%q): %v", tc.url, err)
			}
			if got != tc.want {
				t.Errorf("parseRemote(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

// GitHub Enterprise is out of scope for this phase. A remote pointing
// somewhere else has to say so rather than be read as a github.com
// repository that does not exist.
func TestParseRemoteRefusesAnythingButGitHubCom(t *testing.T) {
	t.Parallel()

	for _, url := range []string{
		"git@gitlab.com:kukv/octoscope.git",
		"https://github.example.com/kukv/octoscope.git",
		"https://github.com/kukv",
		"",
	} {
		if got, err := parseRemote(url); err == nil {
			t.Errorf("parseRemote(%q) = %q, want an error", url, got)
		}
	}
}

// --repo is an override for the moment; the remote is what the directory is.
func TestAnExplicitRepositoryWinsOverTheRemote(t *testing.T) {
	t.Parallel()

	c := New("", "other/repo", "token")
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("git must not run when the repository was named")
		return nil, nil
	}
	vars, err := c.repoVars("")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	assertRepoVars(t, vars, "other", "repo")
}

// The per-call repository wins over the client's: the Repos sidebar fetches
// several repositories through one client.
func TestAPerCallRepositoryWinsOverTheClients(t *testing.T) {
	t.Parallel()

	c := New("", "other/repo", "token")
	vars, err := c.repoVars("kukv/octoscope")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	assertRepoVars(t, vars, "kukv", "octoscope")
}

func TestTheRemoteFillsInTheRepositoryWhenNoneWasNamed(t *testing.T) {
	t.Parallel()

	c := New("/somewhere", "", "token")
	var gotDir string
	var gotArgs []string
	c.runGit = func(_ context.Context, dir string, args ...string) ([]byte, error) {
		gotDir, gotArgs = dir, args
		return []byte("git@github.com:kukv/octoscope.git\n"), nil
	}
	vars, err := c.repoVars("")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	assertRepoVars(t, vars, "kukv", "octoscope")
	if gotDir != "/somewhere" {
		t.Errorf("dir = %q, want /somewhere", gotDir)
	}
	// "-v" or any other flag would return something this code cannot parse.
	if want := []string{"remote", "get-url", "origin"}; !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
}

// A directory that is no repository leaves the Repos tab without a current
// one, which the app reads as "there is none". It must not be fatal.
func TestADirectoryThatIsNoRepositoryIsAnOrdinaryFailure(t *testing.T) {
	t.Parallel()

	c := New("/tmp", "", "token")
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("fatal: not a git repository")
	}
	_, err := c.repoVars("")
	if err == nil {
		t.Fatal("want an error")
	}
	if gh.IsFatal(err) {
		t.Fatal("a directory with no repository must not cost the user their screen")
	}
}

// The remote is read once: the sidebar asks for several repositories at
// once, and a subprocess per badge would be paid for every refresh.
func TestTheRemoteIsReadOnlyOnce(t *testing.T) {
	t.Parallel()

	c := New("", "", "token")
	calls := 0
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		calls++
		return []byte("https://github.com/kukv/octoscope.git"), nil
	}
	for range 3 {
		if _, err := c.repoVars(""); err != nil {
			t.Fatalf("repoVars: %v", err)
		}
	}
	if calls != 1 {
		t.Errorf("git ran %d times, want 1", calls)
	}
}

// A lookup git actually answered -- even with an error, such as no origin
// remote configured -- is permanent: asking again would not change it.
func TestAnOrdinaryGitFailureIsCachedNotRetried(t *testing.T) {
	t.Parallel()

	c := New("", "", "token")
	calls := 0
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		calls++
		return nil, errors.New("fatal: no such remote 'origin'")
	}
	for range 2 {
		if _, err := c.repoVars(""); err == nil {
			t.Fatal("want an error")
		}
	}
	if calls != 1 {
		t.Errorf("git ran %d times, want 1", calls)
	}
}

// A lookup killed by this package's own deadline is a different thing from
// "there is no origin remote": the next attempt may well succeed, so it must
// not be remembered for the client's lifetime the way a settled answer is.
//
// This test shortens remoteTimeout rather than waiting on the real one, and
// has the fake wait on ctx.Done() rather than sleeping a fixed duration, so
// it is exact about what "killed by the deadline" means without being slow
// or flaky. It cannot run in parallel with other tests: it mutates the
// package-level remoteTimeout.
func TestARemoteReadKilledByTheDeadlineIsRetried(t *testing.T) {
	orig := remoteTimeout
	remoteTimeout = 10 * time.Millisecond
	t.Cleanup(func() { remoteTimeout = orig })

	c := New("", "", "token")
	calls := 0
	c.runGit = func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		<-ctx.Done()
		return nil, ctx.Err()
	}
	for range 2 {
		if _, err := c.repoVars(""); err == nil {
			t.Fatal("want an error")
		}
	}
	if calls != 2 {
		t.Errorf("git ran %d times, want 2: a killed lookup must be retried", calls)
	}
}

func assertRepoVars(t *testing.T, vars []gql.Var, owner, name string) {
	t.Helper()

	got := map[string]string{}
	for _, v := range vars {
		if v.Kind != gql.VarString {
			t.Errorf("%s is %v, want VarString", v.Name, v.Kind)
		}
		got[v.Name] = v.Str
	}
	if got["owner"] != owner || got["name"] != name {
		t.Errorf("vars = %v, want owner=%s name=%s", got, owner, name)
	}
}

// TestListPRsInTheWorkingDirectorysRepo is the ordinary case: no --repo, so
// the repository comes from the checkout's git remote rather than from a
// string that can be split. This guards the wiring between Client and
// gql.Client.RepoVars: without it every call falls back to
// gql.SplitRepoVars, which cannot answer for an empty repository, and each
// one fails with `name repository: repo "" has no owner/name separator`.
func TestListPRsInTheWorkingDirectorysRepo(t *testing.T) {
	t.Parallel()

	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequests":{"nodes":[]}}}}`)
	}))
	t.Cleanup(srv.Close)

	c := New("/w", "", "secret-token")
	c.baseURL = srv.URL
	c.runGit = func(context.Context, string, ...string) ([]byte, error) {
		return []byte("git@github.com:kukv/octoscope.git\n"), nil
	}
	if _, err := c.ListPRs(context.Background(), ""); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}

	var sent struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if sent.Variables["owner"] != "kukv" {
		t.Errorf("owner = %v, want kukv", sent.Variables["owner"])
	}
	if sent.Variables["name"] != "octoscope" {
		t.Errorf("name = %v, want octoscope", sent.Variables["name"])
	}
}
