package cli

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

const prListJSON = `[{"number":12,"title":"feat: add pane view","author":{"is_bot":false,"login":"kukv"},"state":"OPEN","isDraft":false,"updatedAt":"2026-07-11T10:00:00Z","reviewDecision":"APPROVED","url":"https://github.com/kukv/demo/pull/12"}]`

const prViewJSON = `{"number":12,"title":"feat: add pane view","author":{"is_bot":false,"login":"kukv"},"state":"OPEN","isDraft":false,"updatedAt":"2026-07-11T10:00:00Z","reviewDecision":"REVIEW_REQUIRED","url":"https://github.com/kukv/demo/pull/12","body":"Adds the pane.","labels":[{"id":"LA_x","name":"Kind: Feature","description":"","color":"ededed"}],"comments":[{"author":{"login":"bob"},"body":"LGTM","createdAt":"2026-07-11T11:00:00Z"}]}`

const issueListJSON = `[{"number":3,"title":"bug: crash on empty list","author":{"is_bot":false,"login":"alice"},"state":"OPEN","updatedAt":"2026-07-10T09:00:00Z","labels":[],"url":"https://github.com/kukv/demo/issues/3"}]`

const issueViewJSON = `{"number":3,"title":"bug: crash on empty list","author":{"is_bot":false,"login":"alice"},"state":"OPEN","updatedAt":"2026-07-10T09:00:00Z","labels":[],"url":"https://github.com/kukv/demo/issues/3","body":"Steps to reproduce.","comments":[{"author":{"login":"carol"},"body":"Confirmed","createdAt":"2026-07-10T10:00:00Z"}]}`

// fakeRun records invocations and returns canned output.
type fakeRun struct {
	dir  string
	args []string
	out  []byte
	err  error
}

func (f *fakeRun) run(_ context.Context, dir string, args ...string) ([]byte, error) {
	f.dir = dir
	f.args = args
	return f.out, f.err
}

func newTestClient(out string, err error) (*Client, *fakeRun) {
	f := &fakeRun{out: []byte(out), err: err}
	return &Client{dir: "/repo", run: f.run}, f
}

func TestListPRs(t *testing.T) {
	c, f := newTestClient(prListJSON, nil)
	prs, err := c.ListPRs(t.Context())
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	wantArgs := []string{"pr", "list", "--json", prListFields}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if f.dir != "/repo" {
		t.Errorf("dir = %q, want /repo", f.dir)
	}
	// The domain value, not the string GitHub sent: the translation is what
	// this layer exists to do (.claude/rules/architecture.md).
	if len(prs) != 1 || prs[0].Number != 12 || prs[0].Author.Login != "kukv" ||
		prs[0].Review != gh.ReviewApproved || prs[0].State != gh.StateOpen {
		t.Errorf("unexpected parse result: %+v", prs)
	}
}

func TestListPRsEmpty(t *testing.T) {
	c, _ := newTestClient(`[]`, nil)
	prs, err := c.ListPRs(t.Context())
	if err != nil || len(prs) != 0 {
		t.Errorf("prs = %v, err = %v; want empty, nil", prs, err)
	}
}

func TestGetPRParsesDetailFields(t *testing.T) {
	c, f := newTestClient(prViewJSON, nil)
	pr, err := c.GetPR(t.Context(), "", 12)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	wantArgs := []string{"pr", "view", "12", "--json", prViewFields}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if pr.Body != "Adds the pane." || len(pr.Comments) != 1 ||
		pr.Comments[0].Author.Login != "bob" || len(pr.Labels) != 1 ||
		pr.Labels[0].Name != "Kind: Feature" {
		t.Errorf("unexpected parse result: %+v", pr)
	}
}

func TestGetPRWithRepoOverride(t *testing.T) {
	c, f := newTestClient(prViewJSON, nil)
	if _, err := c.GetPR(t.Context(), "octo/hello", 12); err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	wantArgs := []string{"pr", "view", "12", "--json", prViewFields, "--repo", "octo/hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestListIssues(t *testing.T) {
	c, f := newTestClient(issueListJSON, nil)
	issues, err := c.ListIssues(t.Context())
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	wantArgs := []string{"issue", "list", "--json", issueListFields}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if len(issues) != 1 || issues[0].Number != 3 || issues[0].Author.Login != "alice" {
		t.Errorf("unexpected parse result: %+v", issues)
	}
}

func TestGetIssueWithRepoOverride(t *testing.T) {
	c, f := newTestClient(issueViewJSON, nil)
	issue, err := c.GetIssue(t.Context(), "octo/hello", 3)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	wantArgs := []string{"issue", "view", "3", "--json", issueViewFields, "--repo", "octo/hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if issue.Body != "Steps to reproduce." || len(issue.Comments) != 1 ||
		issue.Comments[0].Author.Login != "carol" {
		t.Errorf("unexpected parse result: %+v", issue)
	}
}

func TestRepoName(t *testing.T) {
	c, f := newTestClient(`{"nameWithOwner":"kukv/demo"}`, nil)
	name, err := c.RepoName(t.Context())
	if err != nil || name != "kukv/demo" {
		t.Errorf("name = %q, err = %v; want kukv/demo, nil", name, err)
	}
	wantArgs := []string{"repo", "view", "--json", "nameWithOwner"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestOpenPRWebWithRepoOverride(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.OpenPRWeb("octo/hello", 7); err != nil {
		t.Fatalf("OpenPRWeb: %v", err)
	}
	wantArgs := []string{"pr", "view", "7", "--web", "--repo", "octo/hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestOpenIssueWebWithRepoOverride(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.OpenIssueWeb("octo/hello", 3); err != nil {
		t.Fatalf("OpenIssueWeb: %v", err)
	}
	wantArgs := []string{"issue", "view", "3", "--web", "--repo", "octo/hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestRunErrorPassesThrough(t *testing.T) {
	wantErr := errors.New("gh pr: no git remotes found")
	c, _ := newTestClient("", wantErr)
	if _, err := c.ListPRs(t.Context()); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestAddPRComment(t *testing.T) {
	c, f := newTestClient("https://github.com/kukv/demo/pull/12#issuecomment-1\n", nil)
	if err := c.AddPRComment("", 12, "hello"); err != nil {
		t.Fatalf("AddPRComment: %v", err)
	}
	wantArgs := []string{"pr", "comment", "12", "--body", "hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if f.dir != "/repo" {
		t.Errorf("dir = %q, want /repo", f.dir)
	}
}

func TestAddIssueCommentWithRepoOverride(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.AddIssueComment("octo/hello", 3, "hi there"); err != nil {
		t.Fatalf("AddIssueComment: %v", err)
	}
	wantArgs := []string{"issue", "comment", "3", "--body", "hi there", "--repo", "octo/hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestAddCommentError(t *testing.T) {
	wantErr := errors.New("gh pr: HTTP 403 forbidden")
	c, _ := newTestClient("", wantErr)
	if err := c.AddPRComment("", 12, "x"); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestClosePR(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.ClosePR("", 12); err != nil {
		t.Fatalf("ClosePR: %v", err)
	}
	wantArgs := []string{"pr", "close", "12"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if f.dir != "/repo" {
		t.Errorf("dir = %q, want /repo", f.dir)
	}
}

func TestReopenIssueWithRepoOverride(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.ReopenIssue("octo/hello", 3); err != nil {
		t.Fatalf("ReopenIssue: %v", err)
	}
	wantArgs := []string{"issue", "reopen", "3", "--repo", "octo/hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestStateChangeError(t *testing.T) {
	wantErr := errors.New("gh pr: HTTP 403 forbidden")
	c, _ := newTestClient("", wantErr)
	if err := c.ClosePR("", 12); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestListLabels(t *testing.T) {
	c, f := newTestClient(`[{"name":"bug","color":"d73a4a"},{"name":"wip","color":"ededed"}]`, nil)
	labels, err := c.ListLabels(t.Context(), "")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	wantArgs := []string{"label", "list", "--json", "name,color", "--limit", "100"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if len(labels) != 2 || labels[0].Name != "bug" || labels[0].Color != "d73a4a" || labels[1].Name != "wip" {
		t.Errorf("unexpected parse result: %+v", labels)
	}
}

func TestListAssignees(t *testing.T) {
	c, f := newTestClient(`[{"login":"alice"},{"login":"bob"}]`, nil)
	users, err := c.ListAssignees(t.Context(), "")
	if err != nil {
		t.Fatalf("ListAssignees: %v", err)
	}
	wantArgs := []string{"api", "repos/{owner}/{repo}/assignees?per_page=100"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
	if len(users) != 2 || users[0] != "alice" || users[1] != "bob" {
		t.Errorf("unexpected parse result: %v", users)
	}
}

func TestListAssigneesWithRepoOverride(t *testing.T) {
	c, f := newTestClient(`[]`, nil)
	if _, err := c.ListAssignees(t.Context(), "octo/hello"); err != nil {
		t.Fatalf("ListAssignees: %v", err)
	}
	wantArgs := []string{"api", "repos/octo/hello/assignees?per_page=100"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestGetPRParsesAssignees(t *testing.T) {
	c, _ := newTestClient(`{"number":12,"title":"t","assignees":[{"login":"alice"},{"login":"bob"}]}`, nil)
	pr, err := c.GetPR(t.Context(), "", 12)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if len(pr.Assignees) != 2 || pr.Assignees[0].Login != "alice" || pr.Assignees[1].Login != "bob" {
		t.Errorf("unexpected assignees: %+v", pr.Assignees)
	}
}

func TestEditPRLabels(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.EditPRLabels("", 12, []string{"bug"}, []string{"wip"}); err != nil {
		t.Fatalf("EditPRLabels: %v", err)
	}
	wantArgs := []string{"pr", "edit", "12", "--add-label", "bug", "--remove-label", "wip"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestEditPRLabelsAddOnly(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.EditPRLabels("", 12, []string{"a", "b"}, nil); err != nil {
		t.Fatalf("EditPRLabels: %v", err)
	}
	wantArgs := []string{"pr", "edit", "12", "--add-label", "a", "--add-label", "b"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestEditIssueAssigneesWithRepoOverride(t *testing.T) {
	c, f := newTestClient("", nil)
	if err := c.EditIssueAssignees("octo/hello", 3, []string{"alice"}, []string{"bob"}); err != nil {
		t.Fatalf("EditIssueAssignees: %v", err)
	}
	wantArgs := []string{"issue", "edit", "3", "--add-assignee", "alice", "--remove-assignee", "bob", "--repo", "octo/hello"}
	if !reflect.DeepEqual(f.args, wantArgs) {
		t.Errorf("args = %v, want %v", f.args, wantArgs)
	}
}

func TestEditItemsError(t *testing.T) {
	wantErr := errors.New("gh pr: HTTP 403 forbidden")
	c, _ := newTestClient("", wantErr)
	if err := c.EditPRLabels("", 12, []string{"bug"}, nil); !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestClientUsesDefaultRepo(t *testing.T) {
	var got []string
	c := New("/tmp", "kukv/octoscope")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.ListPRs(t.Context()); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	want := []string{"pr", "list", "--json", prListFields, "--repo", "kukv/octoscope"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestPerCallRepoOverridesDefault(t *testing.T) {
	var got []string
	c := New("/tmp", "kukv/octoscope")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("{}"), nil
	}
	if _, err := c.GetPR(t.Context(), "herdr/herdr", 7); err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if !slices.Contains(got, "herdr/herdr") || slices.Contains(got, "kukv/octoscope") {
		t.Errorf("args = %v, want the per-call repo to win", got)
	}
}

func TestRepoNameUsesPositionalArgument(t *testing.T) {
	var got []string
	c := New("/tmp", "kukv/octoscope")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"nameWithOwner":"kukv/octoscope"}`), nil
	}
	if _, err := c.RepoName(t.Context()); err != nil {
		t.Fatalf("RepoName: %v", err)
	}
	want := []string{"repo", "view", "kukv/octoscope", "--json", "nameWithOwner"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestListAssigneesBuildsAPIPathFromDefaultRepo(t *testing.T) {
	var got []string
	c := New("/tmp", "kukv/octoscope")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte("[]"), nil
	}
	if _, err := c.ListAssignees(t.Context(), ""); err != nil {
		t.Fatalf("ListAssignees: %v", err)
	}
	want := []string{"api", "repos/kukv/octoscope/assignees?per_page=100"}
	if !slices.Equal(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}
