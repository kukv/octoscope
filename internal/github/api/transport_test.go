package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
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
	c.baseURL = srv.URL
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
		if !errors.Is(err, github.ErrTransient) {
			t.Errorf("status %d: err = %v, want ErrTransient", status, err)
		}
	}
}

func TestABadCredentialIsUnauthenticated(t *testing.T) {
	t.Parallel()

	c, _ := serve(t, http.StatusUnauthorized, `{"message":"Bad credentials"}`)
	_, err := c.post(context.Background(), "query {}", nil)
	if !errors.Is(err, github.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
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
	if errors.Is(err, github.ErrUnauthenticated) || errors.Is(err, github.ErrNotInstalled) {
		t.Error("a rate limit is not something the user has to fix before anything works")
	}
}

// A server that accepts the connection and then never answers would otherwise
// hold the screen forever: there is no deadline on the context the TUI passes,
// and gh is not here to be killed.
func TestAServerThatNeverAnswersDoesNotHangForever(t *testing.T) {
	orig := responseTimeout
	responseTimeout = 20 * time.Millisecond
	t.Cleanup(func() { responseTimeout = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	t.Cleanup(srv.Close)

	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL

	done := make(chan error, 1)
	go func() {
		_, err := c.read(context.Background(), "repos/kukv/octoscope/labels", "")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read: want an error when the server never answers")
		}
	case <-time.After(time.Second):
		t.Fatal("read did not give up")
	}
}

// The body of a large answer is not on the clock the headers are. A run's log
// archive is megabytes: cutting it off part way would show half a log as if
// the job had printed half a log.
func TestASlowBodyIsNotCutOff(t *testing.T) {
	orig := responseTimeout
	responseTimeout = 50 * time.Millisecond
	t.Cleanup(func() { responseTimeout = orig })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		time.Sleep(150 * time.Millisecond)
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(srv.Close)

	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL

	if _, err := c.read(context.Background(), "repos/kukv/octoscope/labels", ""); err != nil {
		t.Fatalf("read: %v, want the slow body to arrive", err)
	}
}

// shorten puts both limits on a test's clock, so a stall is seen in
// milliseconds rather than in the sixteen seconds the real ones allow. New
// reads responseTimeout, so this runs before the client is built.
func shorten(t *testing.T, stall, response time.Duration) {
	t.Helper()
	origStall, origResponse := stallTimeout, responseTimeout
	stallTimeout, responseTimeout = stall, response
	t.Cleanup(func() { stallTimeout, responseTimeout = origStall, origResponse })
}

// hanging stands in for a GitHub whose line goes quiet. start runs first and
// may send part of an answer; then the handler waits until the client gives
// up or the test ends, so srv.Close is not left waiting on it. It counts the
// requests it was sent.
func hanging(t *testing.T, start func(http.ResponseWriter)) (*Client, *atomic.Int32) {
	t.Helper()

	var sent atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		start(w)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	// Cleanups run last-registered first: the handlers are released before
	// srv.Close waits on them.
	t.Cleanup(func() { close(release) })

	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL
	return c, &sent
}

// partOfABody answers with headers and the first byte of a body, then stops.
func partOfABody(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `[`)
	w.(http.Flusher).Flush()
}

// within runs call and fails the test if it has not returned in two seconds,
// far past any limit these tests shorten to.
func within(t *testing.T, call func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- call() }()
	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("the call did not return within two seconds")
		return nil
	}
}

// wantTimedOut checks the one shape every limit here gives up with:
// transient, so the UI shows it on the one-line notice, and a deadline, so a
// read path does not ask again and double the wait.
func wantTimedOut(t *testing.T, err error, limit time.Duration) {
	t.Helper()
	if err == nil {
		t.Fatal("err = nil, want the request to time out")
	}
	if !errors.Is(err, github.ErrTransient) {
		t.Errorf("err = %v, want it transient", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want it a deadline", err)
	}
	if !strings.Contains(err.Error(), limit.String()) {
		t.Errorf("err = %q, want it to name the limit %s", err, limit)
	}
}

// Headers on time and then nothing would otherwise hold the screen forever:
// the header limit is over once they arrive, and the TUI's context has no
// deadline.
func TestARestAnswerThatStopsArrivingIsCutOff(t *testing.T) {
	shorten(t, 50*time.Millisecond, 10*time.Second)
	c, sent := hanging(t, partOfABody)

	err := within(t, func() error {
		_, err := c.read(context.Background(), "repos/kukv/octoscope/labels", "")
		return err
	})
	wantTimedOut(t, err, stallTimeout)
	if n := sent.Load(); n != 1 {
		t.Errorf("requests = %d, want 1: a line that stalled once is not asked again", n)
	}
}

func TestAGraphQLAnswerThatStopsArrivingIsCutOff(t *testing.T) {
	shorten(t, 50*time.Millisecond, 10*time.Second)
	c, _ := hanging(t, partOfABody)

	err := within(t, func() error {
		_, err := c.post(context.Background(), "query { viewer { login } }", nil)
		return err
	})
	wantTimedOut(t, err, stallTimeout)
}

// The stall limit is on the gaps, not on the whole body: a log archive that
// keeps arriving, however slowly, is not cut off part way.
func TestABodyThatKeepsArrivingIsNotCutOffHoweverLongItTakes(t *testing.T) {
	shorten(t, 200*time.Millisecond, 10*time.Second)

	// 40 bytes 20ms apart take 800ms, four times the stall limit. Each gap is
	// a tenth of the limit, so a loaded runner does not see one as a stall.
	want := strings.Repeat("x", 40)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		for _, b := range []byte(want) {
			_, _ = w.Write([]byte{b})
			w.(http.Flusher).Flush()
			time.Sleep(20 * time.Millisecond)
		}
	}))
	t.Cleanup(srv.Close)
	c := New("", "kukv/octoscope", "secret-token")
	c.baseURL = srv.URL

	got, err := c.read(context.Background(), "repos/kukv/octoscope/labels", "")
	if err != nil {
		t.Fatalf("read: %v, want the whole body", err)
	}
	if string(got) != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

// No headers in time is the same give-up as a stalled body, and is not asked
// again either.
func TestNoHeadersInTimeIsATimeoutAskedOnce(t *testing.T) {
	shorten(t, 10*time.Second, 50*time.Millisecond)
	c, sent := hanging(t, func(http.ResponseWriter) {})

	err := within(t, func() error {
		_, err := c.read(context.Background(), "repos/kukv/octoscope/labels", "")
		return err
	})
	wantTimedOut(t, err, responseTimeout)
	if n := sent.Load(); n != 1 {
		t.Errorf("requests = %d, want 1: a line that stalled once is not asked again", n)
	}
}

// A request the caller stopped is not GitHub failing: it must not land on the
// notice as a transient error, whichever part of the answer it was waiting on.
func TestACallerThatStopsWaitingIsNotATransientFailure(t *testing.T) {
	cases := []struct {
		name  string
		start func(http.ResponseWriter)
		ctx   func() (context.Context, context.CancelFunc)
	}{
		{"cancelled before the headers", func(http.ResponseWriter) {}, cancelSoon},
		{"deadline before the headers", func(http.ResponseWriter) {}, deadlineSoon},
		{"cancelled during the body", partOfABody, cancelSoon},
		{"deadline during the body", partOfABody, deadlineSoon},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shorten(t, 10*time.Second, 10*time.Second)
			c, _ := hanging(t, tc.start)
			ctx, cancel := tc.ctx()
			defer cancel()

			err := within(t, func() error {
				_, err := c.read(ctx, "repos/kukv/octoscope/labels", "")
				return err
			})
			if err == nil {
				t.Fatalf("%s: err = nil, want the stopped request to fail", tc.name)
			}
			if errors.Is(err, github.ErrTransient) {
				t.Errorf("%s: err = %v, want it not transient: the caller stopped it", tc.name, err)
			}
		})
	}
}

func cancelSoon() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(30*time.Millisecond, cancel)
	return ctx, cancel
}

func deadlineSoon() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Millisecond)
}
