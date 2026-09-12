// Package api fetches GitHub data by talking to the API itself, for machines
// that have no gh CLI. It answers the same domain types internal/gh/cli does
// and sends the same GraphQL documents, through a different transport.
package api

import (
	"net/http"
	"os"
	"strings"

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
	repo  string
	token string
	// endpoint and client are the seams the tests use: a local server, and
	// a client with a shorter patience than the default.
	endpoint string
	client   *http.Client
}

// New returns a client for the repository named by repo ("owner/name").
func New(repo, token string) *Client {
	c := &Client{repo: repo, token: token}
	c.Client = &gql.Client{Do: c.post}
	return c
}

// endpointURL is where the documents go. Tests point it at a local server.
func (c *Client) endpointURL() string {
	if c.endpoint != "" {
		return c.endpoint
	}
	return defaultEndpoint
}

// httpClient is the client to send with. Callers do not set one.
func (c *Client) httpClient() *http.Client {
	if c.client != nil {
		return c.client
	}
	return http.DefaultClient
}

// OpenWeb shows the item in a browser. It is the same call the cli backend
// makes: GitHub gives every item its URL, and opening one needs no backend.
func (c *Client) OpenWeb(url string) error {
	return browser.Open(url)
}
