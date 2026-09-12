package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// endpoint is github.com's GraphQL endpoint. GitHub Enterprise is out of
// scope for this phase, so GH_HOST is not read.
const defaultEndpoint = "https://api.github.com/graphql"

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

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("reach GitHub: %w", err)
	}
	// Closing a body that was only read has nothing to report.
	defer func() { _ = resp.Body.Close() }()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read answer: %w", err)
	}
	return out, classify(resp.StatusCode, out)
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

// classify names the failures a caller acts on differently: one worth asking
// again for, and one only the user can fix. Everything else keeps what
// GitHub said and no type at all.
func classify(status int, body []byte) error {
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
		return gh.Classify(gh.ErrTransient, msg)
	case status == http.StatusUnauthorized:
		if msg == "" {
			msg = "HTTP 401"
		}
		return gh.Classify(gh.ErrUnauthenticated, msg)
	case status >= 400:
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return gh.Classify(nil, msg)
	case msg != "":
		// 200 with a top-level errors array: a partially resolvable query.
		return gh.Classify(nil, msg)
	}
	return nil
}
