package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

// jsonMediaType is what GitHub asks REST clients to send. A call that wants
// something else -- a diff, say -- names its own.
const jsonMediaType = "application/vnd.github+json"

// apiVersion pins the REST shape. Without it GitHub is free to serve a newer
// one, and the decoding here is written against this version.
const apiVersion = "2022-11-28"

// restURL turns a path into an absolute URL against this client's base.
func (c *Client) restURL(path string) string {
	return c.base() + "/" + strings.TrimPrefix(path, "/")
}

// send makes one REST request. url is absolute: a paging caller gets the next
// one from the Link header rather than building it.
//
// It returns the body even when err is non-nil, the way post does: a caller
// that wants GitHub's own words has them, and a partial answer is not thrown
// away before anyone has looked at it.
func (c *Client) send(ctx context.Context, method, url string, body any, accept string) ([]byte, http.Header, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, nil, fmt.Errorf("build request body: %w", err)
		}
		payload = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, payload)
	if err != nil {
		return nil, nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("User-Agent", "octoscope")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	if accept == "" {
		accept = jsonMediaType
	}
	req.Header.Set("Accept", accept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("reach GitHub: %w", err)
	}
	// Closing a body that was only read has nothing to report.
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.Header, fmt.Errorf("read answer: %w", err)
	}
	return out, resp.Header, statusError(resp.StatusCode, out)
}

// readURL fetches one absolute URL, asking again once when GitHub's front end
// did not answer. Only reads take this path: a 502 says no answer came back,
// not that nothing arrived, so a repeated write could apply twice.
//
// It takes the URL rather than a path so a paging caller can retry a later
// page too: read alone only ever asked for the first one.
func (c *Client) readURL(ctx context.Context, url, accept string) ([]byte, http.Header, error) {
	out, header, err := c.send(ctx, http.MethodGet, url, nil, accept)
	if err == nil || ctx.Err() != nil || !errors.Is(err, gh.ErrTransient) {
		return out, header, err
	}
	return c.send(ctx, http.MethodGet, url, nil, accept)
}

// read fetches one path. See readURL for the retry.
func (c *Client) read(ctx context.Context, path, accept string) ([]byte, error) {
	out, _, err := c.readURL(ctx, c.restURL(path), accept)
	return out, err
}

// write sends one change. It is never repeated.
func (c *Client) write(ctx context.Context, method, path string, body any) ([]byte, error) {
	out, _, err := c.send(ctx, method, c.restURL(path), body, "")
	return out, err
}

// repoPath is the "owner/name" every REST path is built from. An explicit
// repository wins, then the client's own, then the working directory's
// remote: the same order repoVars uses for GraphQL.
func (c *Client) repoPath(repo string) (string, error) {
	if repo == "" {
		repo = c.repo
	}
	if repo == "" {
		var err error
		if repo, err = c.currentRepo(); err != nil {
			return "", err
		}
	}
	if _, _, ok := gh.SplitRepo(repo); !ok {
		return "", fmt.Errorf("repo %q has no owner/name separator", repo)
	}
	return repo, nil
}

// maxPages caps a paged read at 50 pages, i.e. 5000 entries at pageSize --
// comfortably past anything this backend ever needs to page through. It also
// bounds how long a next link that points back at itself can loop for: send
// always attaches this client's own token to whatever URL it is given, so a
// next link is not trusted to eventually run out on its own.
const maxPages = 50

// walkPages fetches path and then whatever the response's Link header names
// as the next page, feeding each page's raw body to each, until there is no
// next page, maxPages is reached, or the next link fails validNext.
//
// Both callers page a JSON list, so this always reads with the default
// Accept rather than taking one -- a paged diff media type request has never
// come up.
func (c *Client) walkPages(ctx context.Context, path string, each func([]byte) error) error {
	next := c.restURL(path)
	for i := 0; next != "" && i < maxPages; i++ {
		out, header, err := c.readURL(ctx, next, "")
		if err != nil {
			return err
		}
		if err := each(out); err != nil {
			return err
		}
		next = validNext(nextLink(header), c.base())
	}
	return nil
}

// validNext rejects a next link whose scheme or host does not match base,
// treating it as if there were no next page rather than as an error: a next
// link is echoed straight into an Authorization header by send, so it is not
// followed on trust alone.
func validNext(next, base string) string {
	if next == "" {
		return ""
	}
	b, err := url.Parse(base)
	if err != nil {
		return ""
	}
	n, err := url.Parse(next)
	if err != nil || n.Scheme != b.Scheme || n.Host != b.Host {
		return ""
	}
	return next
}

// nextLink is the URL of the page after this one, or empty on the last page.
// GitHub puts every relation in one Link header, so the rel has to be read
// rather than the position.
func nextLink(h http.Header) string {
	for _, part := range strings.Split(h.Get("Link"), ",") {
		fields := strings.Split(part, ";")
		if len(fields) < 2 {
			continue
		}
		url := strings.TrimSpace(fields[0])
		if !strings.HasPrefix(url, "<") || !strings.HasSuffix(url, ">") {
			continue
		}
		for _, f := range fields[1:] {
			if strings.TrimSpace(f) == `rel="next"` {
				return url[1 : len(url)-1]
			}
		}
	}
	return ""
}
