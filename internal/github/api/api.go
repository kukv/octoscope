// Package api fetches GitHub data by talking to the API itself, for machines
// that have no gh CLI. It answers the same domain types internal/gh/cli does
// and sends the same GraphQL documents, through a different transport.
package api

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/browser"
	"github.com/kukv/octoscope/internal/github/gql"
)

// responseTimeout bounds how long GitHub has to start answering: the time to
// connect, to shake hands, and to send response headers. It does not bound
// reading the body, because one answer here is a run's whole log archive and
// cutting that off part way would look like a job that printed half a log.
//
// Eight seconds is the longest answer measured while this backend was built
// (thirty repository aliases in one GraphQL request, 8.13s including the
// line), so the value is that rounded up to the next power of two: long
// enough that a request which is merely slow still lands.
//
// A var, not a const: tests shorten it to exercise the deadline without
// waiting on it.
var responseTimeout = 16 * time.Second

// Token reads the token this backend authenticates with. The order is gh's
// own: GH_TOKEN wins so that a machine with both can point octoscope at the
// same credential gh uses.
func Token() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, nil
		}
	}
	return "", domain.ErrUnauthenticated
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

	// http is shared by post and send. New builds its Transport from
	// responseTimeout.
	http   *http.Client
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
	c := &Client{
		dir: dir, repo: repo, token: token,
		http: &http.Client{
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				DialContext:           (&net.Dialer{Timeout: responseTimeout}).DialContext,
				TLSHandshakeTimeout:   responseTimeout,
				ResponseHeaderTimeout: responseTimeout,
				ForceAttemptHTTP2:     true,
			},
		},
	}
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
