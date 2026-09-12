package api

import (
	"context"
	"errors"
	"testing"

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
	if len(gotArgs) == 0 || gotArgs[0] != "remote" {
		t.Errorf("args = %v, want a git remote lookup", gotArgs)
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
	if _, err := c.repoVars(""); err == nil {
		t.Fatal("want an error")
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
