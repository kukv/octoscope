// Package api fetches GitHub data by talking to the API itself, for machines
// that have no gh CLI. It answers the same domain types internal/gh/cli does
// and sends the same GraphQL documents, through a different transport.
package api

import (
	"os"
	"strings"
	"sync"

	"github.com/kukv/octoscope/internal/browser"
	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// Token reads the token this backend authenticates with. The order is gh's
// own: GH_TOKEN wins so that a machine with both can point octoscope at the
// same credential gh uses.
func Token() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, nil
		}
	}
	return "", gh.ErrUnauthenticated
}

// Client talks to GitHub over HTTPS. The GraphQL calls come from the
// embedded gql.Client, which this type gives a transport.
type Client struct {
	*gql.Client
	dir   string
	repo  string
	token string
	// baseURL is the seam the tests use: a local server in place of
	// github.com. Both the GraphQL endpoint and every REST path hang off it.
	baseURL string

	runGit gitFunc
	mu     sync.Mutex
	// done is set only once git has actually answered: a lookup killed by
	// its own deadline is worth retrying, not remembering for the client's
	// lifetime.
	done       bool
	current    string
	currentErr error
}

// New returns a client for the repository named by repo ("owner/name"), whose
// working directory is dir. An empty repo is resolved from dir's git remote,
// the way gh resolves it from the working directory.
//
// The three strings are dir, repo, token, in that order: all of them are
// strings, so a swapped pair still compiles.
func New(dir, repo, token string) *Client {
	c := &Client{dir: dir, repo: repo, token: token}
	c.Client = &gql.Client{
		Do:       c.post,
		RepoVars: c.repoVars,
	}
	return c
}

// base is where every request goes. Tests point it at a local server.
func (c *Client) base() string {
	if c.baseURL != "" {
		return c.baseURL
	}
	return defaultBase
}

// endpointURL is where the GraphQL documents go.
func (c *Client) endpointURL() string {
	return c.base() + "/graphql"
}

// OpenWeb shows the item in a browser. It is the same call the cli backend
// makes: GitHub gives every item its URL, and opening one needs no backend.
func (c *Client) OpenWeb(url string) error {
	return browser.Open(url)
}
