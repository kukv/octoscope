package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// recorded is what the stand-in GitHub was sent. The handler fills it while
// the call under test is still running, so the fields are read only after
// that call returns -- a value captured at serve's return would always be
// the zero one.
type recorded struct {
	req  *http.Request
	body []byte
}

// serve stands in for GitHub. It records the one request it was sent.
func serve(t *testing.T, status int, body string) (*Client, *recorded) {
	t.Helper()

	var got recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.req = r.Clone(r.Context())
		got.body, _ = io.ReadAll(r.Body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)

	c := New("", "kukv/octoscope", "secret-token")
	c.endpoint = srv.URL
	return c, &got
}

func TestThePostCarriesTheDocumentAndTheVariables(t *testing.T) {
	t.Parallel()

	c, got := serve(t, http.StatusOK, `{"data":{}}`)
	if _, err := c.post(context.Background(), "query { viewer { login } }",
		[]gql.Var{gql.S("owner", "kukv"), gql.N("number", 61)}); err != nil {
		t.Fatalf("post: %v", err)
	}

	var sent struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(got.body, &sent); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if !strings.Contains(sent.Query, "viewer") {
		t.Errorf("query = %q", sent.Query)
	}
	if sent.Variables["owner"] != "kukv" {
		t.Errorf("owner = %v, want kukv", sent.Variables["owner"])
	}
	// GraphQL rejects "61" where it wants 61: the number has to travel as a
	// JSON number, not as a string.
	if n, ok := sent.Variables["number"].(float64); !ok || n != 61 {
		t.Errorf("number = %#v, want the JSON number 61", sent.Variables["number"])
	}
}

func TestThePostAuthenticatesWithTheToken(t *testing.T) {
	t.Parallel()

	c, got := serve(t, http.StatusOK, `{"data":{}}`)
	if _, err := c.post(context.Background(), "query {}", nil); err != nil {
		t.Fatalf("post: %v", err)
	}
	if h := got.req.Header.Get("Authorization"); h != "bearer secret-token" {
		t.Errorf("Authorization = %q, want bearer secret-token", h)
	}
	// GitHub asks every client to name itself and may refuse an unnamed one.
	if got.req.Header.Get("User-Agent") == "" {
		t.Error("no User-Agent")
	}
}

// A placeholder is a value only gh can fill in, from the working directory's
// remote. Sending "{owner}" to the API would ask GitHub for a repository
// with that literal name.
func TestThePostRefusesAPlaceholder(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusOK, `{"data":{}}`)
	_, err := c.post(context.Background(), "query {}",
		[]gql.Var{gql.Placeholder("owner", "{owner}")})
	if err == nil {
		t.Fatal("want an error for a placeholder variable")
	}
}

func TestAGatewayFailureIsTransient(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		c, _ := serve(t, status, "")
		_, err := c.post(context.Background(), "query {}", nil)
		if !errors.Is(err, gh.ErrTransient) {
			t.Errorf("status %d: err = %v, want ErrTransient", status, err)
		}
	}
}

func TestABadCredentialIsUnauthenticated(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusUnauthorized, `{"message":"Bad credentials"}`)
	_, err := c.post(context.Background(), "query {}", nil)
	if !errors.Is(err, gh.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
	if !gh.IsFatal(err) {
		t.Error("the UI would not show the error screen for this")
	}
}

// GitHub answers a partially resolvable query with the data it could resolve
// beside a top-level errors array. RepoCounts salvages that body to draw the
// badges it does have, so the transport must hand it back with the error.
func TestAPartialAnswerComesBackWithItsBody(t *testing.T) {
	t.Parallel()

	const partial = `{"data":{"r0":{"nameWithOwner":"kukv/octoscope"},"r1":null},` +
		`"errors":[{"type":"NOT_FOUND","message":"Could not resolve to a Repository"}]}`
	c, _ := serve(t, http.StatusOK, partial)

	out, err := c.post(context.Background(), "query {}", nil)
	if err == nil {
		t.Fatal("want an error beside the body")
	}
	if !strings.Contains(string(out), "kukv/octoscope") {
		t.Errorf("body was dropped: %q", out)
	}
	if !strings.Contains(err.Error(), "Could not resolve to a Repository") {
		t.Errorf("err = %q, want GitHub's own words", err)
	}
}

// What GitHub said is the most informative thing a user gets, and the rules
// leave it untranslated. A wrapper that replaces it loses the only actionable
// part.
func TestAnOrdinaryFailureKeepsWhatGitHubSaid(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusForbidden, `{"message":"API rate limit exceeded"}`)
	_, err := c.post(context.Background(), "query {}", nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "API rate limit exceeded") {
		t.Errorf("err = %q, want GitHub's own words", err)
	}
	if gh.IsFatal(err) {
		t.Error("a rate limit is not something the user has to fix before anything works")
	}
}
