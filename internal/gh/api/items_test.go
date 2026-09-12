package api

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
)

// GitHub posts a comment on a pull request through the issues endpoint: a
// pull request is an issue with a branch, and /pulls/{n}/comments is the
// review threads instead. Sending a conversation comment there would put it
// on a diff line.
func TestAPullRequestCommentGoesToTheIssuesEndpoint(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.AddPRComment("cli/cli", 61, "looks good"); err != nil {
		t.Fatalf("AddPRComment: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", req.Method)
	}
	if req.URL.Path != "/repos/cli/cli/issues/61/comments" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent struct {
		Body string `json:"body"`
	}
	body, _ := io.ReadAll(req.Body)
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("parse request: %v", err)
	}
	if sent.Body != "looks good" {
		t.Errorf("body = %q", sent.Body)
	}
}

// Closing a pull request is a different endpoint from closing an issue: the
// issues endpoint would answer for a pull request too, but the pulls one is
// what gh pr close uses and the only one that carries a pull request's own
// fields back.
func TestClosingAPullRequestPatchesThePullsEndpoint(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.ClosePR("cli/cli", 61); err != nil {
		t.Fatalf("ClosePR: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", req.Method)
	}
	if req.URL.Path != "/repos/cli/cli/pulls/61" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent map[string]string
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &sent)
	if sent["state"] != "closed" {
		t.Errorf("state = %q, want closed", sent["state"])
	}
}

func TestReopeningAnIssuePatchesTheIssuesEndpointBackToOpen(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.ReopenIssue("cli/cli", 50); err != nil {
		t.Fatalf("ReopenIssue: %v", err)
	}

	req := (*got)[0]
	if req.URL.Path != "/repos/cli/cli/issues/50" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent map[string]string
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &sent)
	if sent["state"] != "open" {
		t.Errorf("state = %q, want open", sent["state"])
	}
}

// A label's name is a path segment, and GitHub's own labels have spaces
// ("help wanted") and slashes ("area/cli") in them. A space alone would not
// catch a missing escape: net/http re-escapes it to %20 on the way out
// regardless of what built the path. A slash is the case that matters -- left
// unescaped it splits the path into an extra segment instead of naming one
// label.
func TestRemovingALabelEscapesASlashInItsNameInsteadOfSplittingThePath(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if err := c.EditIssueLabels("cli/cli", 50, nil, []string{"help wanted", "area/cli"}); err != nil {
		t.Fatalf("EditIssueLabels: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.Method)
	}
	if req.URL.EscapedPath() != "/repos/cli/cli/issues/50/labels/help%20wanted" {
		t.Errorf("escaped path = %q", req.URL.EscapedPath())
	}

	req = (*got)[1]
	if req.URL.EscapedPath() != "/repos/cli/cli/issues/50/labels/area%2Fcli" {
		t.Errorf("escaped path = %q", req.URL.EscapedPath())
	}
}

// Additions travel as one request and each removal as its own, so a caller
// asking for both must not lose either half.
func TestEditingLabelsSendsTheAdditionsTogetherAndEachRemovalOnItsOwn(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if err := c.EditPRLabels("cli/cli", 61, []string{"bug", "docs"}, []string{"stale", "keep"}); err != nil {
		t.Fatalf("EditPRLabels: %v", err)
	}
	if len(*got) != 3 {
		t.Fatalf("requests = %d, want 3 (one add, two removes)", len(*got))
	}
	var added struct {
		Labels []string `json:"labels"`
	}
	body, _ := io.ReadAll((*got)[0].Body)
	_ = json.Unmarshal(body, &added)
	if len(added.Labels) != 2 {
		t.Errorf("added labels = %v, want both in one request", added.Labels)
	}
}

// Removing assignees carries them in the body, not the path: GitHub takes a
// list there, and the endpoint is the same one additions go to.
func TestRemovingAssigneesCarriesThemInTheBody(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	})
	if err := c.EditIssueAssignees("cli/cli", 50, nil, []string{"octocat"}); err != nil {
		t.Fatalf("EditIssueAssignees: %v", err)
	}

	req := (*got)[0]
	if req.Method != http.MethodDelete {
		t.Errorf("method = %s, want DELETE", req.Method)
	}
	if req.URL.Path != "/repos/cli/cli/issues/50/assignees" {
		t.Errorf("path = %q", req.URL.Path)
	}
	var sent struct {
		Assignees []string `json:"assignees"`
	}
	body, _ := io.ReadAll(req.Body)
	_ = json.Unmarshal(body, &sent)
	if len(sent.Assignees) != 1 || sent.Assignees[0] != "octocat" {
		t.Errorf("assignees = %v", sent.Assignees)
	}
}

// An edit with nothing on one side must not send an empty request: POSTing an
// empty label list is a change GitHub accepts and applies.
func TestAnEditWithNothingToAddSendsNoAddRequest(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[]`)
	})
	if err := c.EditPRLabels("cli/cli", 61, nil, nil); err != nil {
		t.Fatalf("EditPRLabels: %v", err)
	}
	if len(*got) != 0 {
		t.Errorf("requests = %d, want none", len(*got))
	}
}
