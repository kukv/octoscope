// Package api fetches GitHub data by talking to the API itself, for machines
// that have no gh CLI. It answers the same domain types internal/github/cli does
// and sends the same GraphQL documents, through a different transport.
package api

import (
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// responseTimeout bounds how long GitHub has to start answering: the time to
// connect, to shake hands, and to send response headers. It does not bound
// reading the whole body, because one answer here is a run's whole log
// archive and cutting that off part way would look like a job that printed
// half a log; stallTimeout bounds the body instead.
//
// Eight seconds is the longest answer measured while this backend was built
// (thirty repository aliases in one GraphQL request, 8.13s including the
// line), so the value is that rounded up to the next power of two: long
// enough that a request which is merely slow still lands.
//
// A var, not a const: tests shorten it to exercise the deadline without
// waiting on it.
var responseTimeout = 16 * time.Second

// stallTimeout bounds how long the body may go without a single byte
// arriving. It is on the gaps rather than the whole body, so a large archive
// that keeps coming is never cut off. The value is responseTimeout's, for the
// same reason: both are how long GitHub may say nothing at all.
//
// A var for the same reason responseTimeout is one.
var stallTimeout = 16 * time.Second

// answerLimit bounds how much of one answer, or of one log entry unpacked
// from an archive, is read into memory. The largest run archive measured was
// 18 MB (a 38-job matrix); the limit is a little over five times that,
// rounded up to the next power of two, the same margin the cli backend's job
// log deadline leaves.
//
// A var for the same reason responseTimeout is one.
var answerLimit int64 = 128 << 20

// Token reads the token this backend authenticates with. The order is gh's
// own: GH_TOKEN wins so that a machine with both can point octoscope at the
// same credential gh uses.
func Token() (string, error) {
	for _, name := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(name)); v != "" {
			return v, nil
		}
	}
	return "", github.ErrUnauthenticated
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

func (c *Client) endpointURL() string {
	return c.base() + "/graphql"
}
