package api

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// Rerunning everything and rerunning only what failed are two endpoints, not
// one endpoint with a flag. Sending the whole run to the failed-jobs path
// would start jobs that already passed.
func TestRerunScopePicksTheEndpoint(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		scope gh.RerunScope
		want  string
	}{
		{"all", gh.RerunAll, "/repos/kukv/octoscope/actions/runs/61/rerun"},
		{"failed", gh.RerunFailed, "/repos/kukv/octoscope/actions/runs/61/rerun-failed-jobs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `{}`)
			})
			if err := c.RerunWorkflow(context.Background(), "kukv/octoscope", 61, tc.scope); err != nil {
				t.Fatalf("RerunWorkflow: %v", err)
			}

			req := (*got)[0]
			if req.Method != http.MethodPost {
				t.Errorf("method = %s, want POST", req.Method)
			}
			if req.URL.Path != tc.want {
				t.Errorf("path = %q, want %q", req.URL.Path, tc.want)
			}
		})
	}
}

// A rerun is not repeated when GitHub's front end fails to answer: a 502 says
// no answer came back, not that nothing happened, and a second POST would
// start the run twice.
func TestARerunIsNotRepeatedOnATransientFailure(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"message": "Server Error"}`)
	})
	if err := c.RerunWorkflow(context.Background(), "kukv/octoscope", 61, gh.RerunAll); err == nil {
		t.Fatal("RerunWorkflow: want an error")
	}
	if len(*got) != 1 {
		t.Errorf("requests = %d, want 1: a rerun must not be sent twice", len(*got))
	}
}
