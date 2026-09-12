package gql

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// A 502 means no answer came back, not that nothing arrived: the request is
// well-formed, so asking again is the right response.
func TestAReadAsksAgainWhenGitHubDidNotAnswer(t *testing.T) {
	t.Parallel()

	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, gh.Classify(gh.ErrTransient, "HTTP 502")
		}
		return []byte(`{"data":{}}`), nil
	}}
	if _, err := c.Read(context.Background(), "query {}"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if calls != 2 {
		t.Errorf("transport called %d times, want 2", calls)
	}
}

// A write could apply twice. 502 says the answer is missing, not the effect.
func TestAWriteIsNeverSentTwice(t *testing.T) {
	t.Parallel()

	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		return nil, gh.Classify(gh.ErrTransient, "HTTP 502")
	}}
	if _, err := c.Write(context.Background(), "mutation {}"); !errors.Is(err, gh.ErrTransient) {
		t.Fatalf("Write err = %v, want ErrTransient", err)
	}
	if calls != 1 {
		t.Errorf("transport called %d times, want 1", calls)
	}
}

// A cancelled context must not turn into a second request.
func TestACancelledReadStops(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		calls++
		return nil, gh.Classify(gh.ErrTransient, "HTTP 502")
	}}
	if _, err := c.Read(ctx, "query {}"); err == nil {
		t.Fatal("Read succeeded, want an error")
	}
	if calls != 1 {
		t.Errorf("transport called %d times, want 1", calls)
	}
}

// The transport must see the body even when the call failed: a partially
// resolvable query answers with data and errors at once.
func TestTheBodyOfAFailedReadIsReturned(t *testing.T) {
	t.Parallel()

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		return []byte(`{"data":{"r0":null}}`), errors.New("gh api: exit 1")
	}}
	out, err := c.Read(context.Background(), "query {}")
	if err == nil {
		t.Fatal("Read succeeded, want an error")
	}
	if string(out) != `{"data":{"r0":null}}` {
		t.Errorf("body = %q, want the partial body", out)
	}
}

// A Client built with only Do (no RepoVars) still has to name a repository,
// so tests that construct a bare Client{Do: ...} are not forced to also
// stub RepoVars.
func TestRepoVarsFallsBackToSplittingOwnerAndName(t *testing.T) {
	t.Parallel()

	c := &Client{}
	got, err := c.repoVars("kukv/octoscope")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	want := []Var{S("owner", "kukv"), S("name", "octoscope")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repoVars = %+v, want %+v", got, want)
	}
}

// A Client that names a repository some other way (internal/gh/api resolves
// "wherever we are" from the endpoint it talks to, not from a git remote)
// must go through RepoVars instead of the owner/name split.
func TestRepoVarsUsesTheProvidedSplitter(t *testing.T) {
	t.Parallel()

	c := &Client{RepoVars: func(repo string) ([]Var, error) {
		return []Var{S("id", repo)}, nil
	}}
	got, err := c.repoVars("kukv/octoscope")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	want := []Var{S("id", "kukv/octoscope")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("repoVars = %+v, want %+v", got, want)
	}
}
