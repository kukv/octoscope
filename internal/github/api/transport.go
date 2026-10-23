package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// defaultBase is github.com's API root. GitHub Enterprise is out of scope for
// this phase, so GH_HOST is not read.
const defaultBase = "https://api.github.com"

// errorsBody is the part of an answer that says what failed. GitHub puts a
// top-level "errors" array beside whatever "data" it could resolve; a request
// that failed outright carries "message" instead.
type errorsBody struct {
	Message string `json:"message"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// text is the single sentence to show, GitHub's own words and nothing else.
func (b errorsBody) text() string {
	msgs := make([]string, 0, len(b.Errors)+1)
	for _, e := range b.Errors {
		if e.Message != "" {
			msgs = append(msgs, e.Message)
		}
	}
	if len(msgs) == 0 && b.Message != "" {
		msgs = append(msgs, b.Message)
	}
	return strings.Join(msgs, "; ")
}

// post sends one document to GitHub.
//
// It returns the body even when err is non-nil: a partially resolvable query
// answers with the data it could resolve beside a top-level errors array, and
// RepoCounts draws the badges it does have out of that body.
func (c *Client) post(ctx context.Context, doc string, vars []gql.Var) ([]byte, error) {
	payload, err := requestBody(doc, vars)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpointURL(), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	// GitHub asks every client to name itself; an unnamed one may be refused.
	req.Header.Set("User-Agent", "octoscope")

	status, _, out, err := c.exchange(req)
	if err != nil {
		return nil, err
	}
	out = caretJSON(out)
	return out, classify(status, out)
}

// timedOut is a request this package gave up on. It reads as transient, the
// way a 502 does, and as a deadline to the read paths, which must not ask
// again: a connection that stalled once is not one a second try will rescue,
// and a retry doubles the wait.
type timedOut struct{ msg string }

func (e *timedOut) Error() string   { return e.msg }
func (e *timedOut) Unwrap() []error { return []error{github.ErrTransient, context.DeadlineExceeded} }

// errStalled is the cause exchange gives its own stall limit, so that one can
// be told apart from a cancellation or deadline the caller set.
var errStalled = errors.New("the answer stopped arriving")

// exchange sends req and reads the whole answer. It hands back the status and
// header rather than the response, whose body it has already read and
// closed; the header comes back even when reading the body failed.
//
// A stall is decided by the context's cause rather than by the error: a
// stopped body read reports "context canceled" whether the caller stopped it
// or the stall limit did.
func (c *Client) exchange(req *http.Request) (int, http.Header, []byte, error) {
	caller := req.Context()
	ctx, cancel := context.WithCancelCause(caller)
	defer cancel(nil)
	req = req.WithContext(ctx)

	resp, err := c.http.Do(req)
	if err != nil {
		// A caller's own deadline is a timeout to net/http too, and is not
		// GitHub failing to answer.
		var nerr net.Error
		if caller.Err() == nil && errors.As(err, &nerr) && nerr.Timeout() {
			return 0, nil, nil, &timedOut{msg: fmt.Sprintf("reach GitHub: no answer in %s", responseTimeout)}
		}
		return 0, nil, nil, fmt.Errorf("reach GitHub: %w", err)
	}
	// Closing a body that was only read has nothing to report.
	defer func() { _ = resp.Body.Close() }()

	limit := stallTimeout
	timer := time.AfterFunc(limit, func() { cancel(errStalled) })
	defer timer.Stop()
	out, err := io.ReadAll(&progress{r: resp.Body, timer: timer, limit: limit})
	if err != nil {
		if errors.Is(context.Cause(ctx), errStalled) {
			return resp.StatusCode, resp.Header, nil, &timedOut{msg: fmt.Sprintf("read answer: nothing arrived for %s", limit)}
		}
		return resp.StatusCode, resp.Header, nil, fmt.Errorf("read answer: %w", err)
	}
	return resp.StatusCode, resp.Header, out, nil
}

// progress restarts timer each time part of the body arrives.
type progress struct {
	r     io.Reader
	timer *time.Timer
	limit time.Duration
}

func (p *progress) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	if n > 0 {
		p.timer.Reset(p.limit)
	}
	return n, err
}

// requestBody spells the document and its variables the way GraphQL takes
// them. A number has to travel as a JSON number: GraphQL rejects "61" where
// it wants 61.
func requestBody(doc string, vars []gql.Var) ([]byte, error) {
	values := make(map[string]any, len(vars))
	for _, v := range vars {
		switch v.Kind {
		case gql.VarInt:
			values[v.Name] = v.Int
		case gql.VarPlaceholder:
			// Only gh fills these in, from the working directory's remote.
			// Sending one would ask GitHub for a repository literally named
			// "{repo}".
			return nil, fmt.Errorf("variable %q is a placeholder only gh can fill in", v.Name)
		default:
			values[v.Name] = v.Str
		}
	}
	payload, err := json.Marshal(map[string]any{"query": doc, "variables": values})
	if err != nil {
		return nil, fmt.Errorf("build request body: %w", err)
	}
	return payload, nil
}

// statusError names the failures a caller acts on differently: one worth
// asking again for, and one only the user can fix. Everything else keeps what
// GitHub said and no type at all.
func statusError(status int, body []byte) error {
	var b errorsBody
	// A body that is not JSON leaves b zero, which reads as "no words from
	// GitHub" -- the status alone then describes the failure.
	_ = json.Unmarshal(body, &b)
	msg := b.text()

	switch {
	case status == http.StatusBadGateway,
		status == http.StatusServiceUnavailable,
		status == http.StatusGatewayTimeout:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return github.Classify(github.ErrTransient, msg)
	case status == http.StatusUnauthorized:
		if msg == "" {
			msg = "HTTP 401"
		}
		return github.Classify(github.ErrUnauthenticated, msg)
	case status >= 400:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return github.Classify(nil, msg)
	}
	return nil
}

// classify is statusError plus the one shape only GraphQL has: HTTP 200 with
// a top-level errors array, which is how a partially resolvable query answers.
func classify(status int, body []byte) error {
	if err := statusError(status, body); err != nil {
		return err
	}
	var b errorsBody
	_ = json.Unmarshal(body, &b)
	if msg := b.text(); msg != "" {
		return github.Classify(nil, msg)
	}
	return nil
}
