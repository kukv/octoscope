package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// REST answers labels by name and carries no creation time, so the order gh
// shows (CREATED_AT ascending) can only come from GraphQL. Listing through
// REST, even sorted by id, was measured putting a label out of place.
func TestLabelsAreAskedOfGraphQLInCreationOrder(t *testing.T) {
	t.Parallel()

	var req *http.Request
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req = r
		body, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"data":{"repository":{"labels":{"nodes":[]}}}}`)
	}))
	t.Cleanup(srv.Close)
	c := New("", "", "secret-token")
	c.baseURL = srv.URL

	if _, err := c.ListLabels(context.Background(), "cli/cli"); err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if req.URL.Path != "/graphql" {
		t.Errorf("path = %q, want /graphql", req.URL.Path)
	}
	var sent struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if !strings.Contains(sent.Query, "field: CREATED_AT, direction: ASC") {
		t.Errorf("query = %q, want the document that orders labels by creation", sent.Query)
	}
	if sent.Variables["owner"] != "cli" || sent.Variables["name"] != "cli" {
		t.Errorf("variables = %v, want owner=cli name=cli", sent.Variables)
	}
}

// The picker shows 100 assignable users at most, the same ceiling the cli
// backend reads. Asking for the default 30 would hide most of a large team.
func TestAssigneesAskForAFullPageNotTheDefaultThirty(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "assignees.json"))
	})
	logins, err := c.ListAssignees(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListAssignees: %v", err)
	}
	if len(logins) == 0 {
		t.Fatal("no logins parsed")
	}
	if logins[0] == "" {
		t.Error("first login is empty")
	}
	if per := (*got)[0].URL.Query().Get("per_page"); per != "100" {
		t.Errorf("per_page = %q, want 100", per)
	}
}
