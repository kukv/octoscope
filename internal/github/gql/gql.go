// Package gql holds the GraphQL documents octoscope sends to GitHub and the
// decoding of their answers. What actually carries a document to GitHub is
// the caller's business: internal/gh/cli runs gh api graphql, internal/gh/api
// posts to the endpoint itself.
package gql

import (
	"context"
	"errors"
	"fmt"

	"github.com/kukv/octoscope/internal/gh"
)

// VarKind is how a transport has to spell one variable.
type VarKind int

const (
	// VarString is an ordinary string variable.
	VarString VarKind = iota
	// VarInt is a number: GraphQL rejects "3" where it wants 3.
	VarInt
	// VarPlaceholder is a value only gh substitutes -- "{owner}" and
	// "{repo}", which it fills from the working directory's remote. A
	// transport that is not gh has to resolve the repository itself before
	// it builds the variables, and never receives one of these.
	VarPlaceholder
)

// Var is one GraphQL variable.
type Var struct {
	Name string
	Kind VarKind
	Str  string
	Int  int
}

// S names a string variable.
func S(name, value string) Var { return Var{Name: name, Kind: VarString, Str: value} }

// N names a number variable.
func N(name string, value int) Var { return Var{Name: name, Kind: VarInt, Int: value} }

// Placeholder names a value gh fills in from the working directory.
func Placeholder(name, value string) Var {
	return Var{Name: name, Kind: VarPlaceholder, Str: value}
}

// Transport sends one document with its variables and returns the response
// body.
//
// It must return the body even when err is non-nil: GitHub answers a
// partially resolvable query with a top-level "errors" array beside the data
// it could resolve, and gh api graphql exits non-zero for that same body.
// RepoCounts is the caller that salvages it; everyone else looks at err.
type Transport func(ctx context.Context, doc string, vars []Var) ([]byte, error)

// Client sends the documents in this package through one transport.
// RepoVars turns "owner/name" (empty for "wherever we are") into the
// variables repository() takes, which the two backends answer differently.
type Client struct {
	Do       Transport
	RepoVars func(repo string) ([]Var, error)
}

// Read sends a query, asking again once when GitHub's front end did not
// answer. Only reads take this path: a 502 says no answer came back, not
// that nothing arrived, so a repeated write could apply twice.
func (c *Client) Read(ctx context.Context, doc string, vars ...Var) ([]byte, error) {
	out, err := c.Do(ctx, doc, vars)
	if err == nil || ctx.Err() != nil || !errors.Is(err, gh.ErrTransient) {
		return out, err
	}
	return c.Do(ctx, doc, vars)
}

// Write sends a mutation. It is never retried.
func (c *Client) Write(ctx context.Context, doc string, vars ...Var) ([]byte, error) {
	return c.Do(ctx, doc, vars)
}

// SplitRepoVars names the repository by splitting "owner/name". It is what
// a transport that cannot fill in a repository of its own gets: GraphQL's
// repository() takes the two halves separately, unlike `gh pr`, which takes
// the whole thing after --repo.
func SplitRepoVars(repo string) ([]Var, error) {
	owner, name, ok := gh.SplitRepo(repo)
	if !ok {
		return nil, fmt.Errorf("repo %q has no owner/name separator", repo)
	}
	return []Var{S("owner", owner), S("name", name)}, nil
}

// repoVars is what a caller uses to name the repository of a call.
func (c *Client) repoVars(repo string) ([]Var, error) {
	split := c.RepoVars
	if split == nil {
		split = SplitRepoVars
	}
	vars, err := split(repo)
	if err != nil {
		return nil, fmt.Errorf("name repository: %w", err)
	}
	return vars, nil
}
