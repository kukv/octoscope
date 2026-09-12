package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

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
	c := New("/repo", "")
	c.run = f.run
	return c, f
}

func readTestdata(t *testing.T, name string) string {
	t.Helper()

	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	return string(b)
}

// The sidebar lists a repository the client was not built for, so the list
// calls have to be able to name one.
func TestListAsksForTheRepositoryItWasGiven(t *testing.T) {
	tests := map[string]func(*Client) ([]string, error){
		"PR": func(c *Client) ([]string, error) {
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(emptyDataJSON), nil
			}
			_, err := c.ListPRs(t.Context(), "cli/cli")
			return got, err
		},
		"issue": func(c *Client) ([]string, error) {
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(emptyDataJSON), nil
			}
			_, err := c.ListIssues(t.Context(), "cli/cli")
			return got, err
		},
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			c := New("/tmp", "kukv/octoscope")
			got, err := call(c)
			if err != nil {
				t.Fatalf("List%s: %v", name, err)
			}
			if repoVarsArg(got) != "cli/cli" {
				t.Errorf("the document names %q, want %q", repoVarsArg(got), "cli/cli")
			}
		})
	}
}

// An empty name still means "the repository this client was built for":
// every existing caller passes nothing.
func TestListFallsBackToTheClientsRepository(t *testing.T) {
	tests := map[string]func(*Client) ([]string, error){
		"PR": func(c *Client) ([]string, error) {
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(emptyDataJSON), nil
			}
			_, err := c.ListPRs(t.Context(), "")
			return got, err
		},
		"issue": func(c *Client) ([]string, error) {
			var got []string
			c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
				got = args
				return []byte(emptyDataJSON), nil
			}
			_, err := c.ListIssues(t.Context(), "")
			return got, err
		},
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			c := New("/tmp", "kukv/octoscope")
			got, err := call(c)
			if err != nil {
				t.Fatalf("List%s: %v", name, err)
			}
			if repoVarsArg(got) != "kukv/octoscope" {
				t.Errorf("the document names %q, want %q", repoVarsArg(got), "kukv/octoscope")
			}
		})
	}
}

// emptyDataJSON is an answer every document decodes into zero values.
const emptyDataJSON = `{"data":{}}`

// repoVarsArg is the repository the document's variables name, spelled the
// way a caller names one, or "" if either half is missing.
func repoVarsArg(args []string) string {
	var owner, name string
	for _, a := range args {
		if rest, ok := strings.CutPrefix(a, "owner="); ok {
			owner = rest
		}
		if rest, ok := strings.CutPrefix(a, "name="); ok {
			name = rest
		}
	}
	if owner == "" || name == "" {
		return ""
	}
	return owner + "/" + name
}

func TestRunErrorPassesThrough(t *testing.T) {
	wantErr := errors.New("gh pr: no git remotes found")
	c, _ := newTestClient("", wantErr)
	if _, err := c.ListPRs(t.Context(), ""); !errors.Is(err, wantErr) {
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
	wantArgs := []string{"label", "list", "--json", "name,color", "--limit", listLimit}
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

func TestPerCallRepoOverridesDefault(t *testing.T) {
	var got []string
	c := New("/tmp", "kukv/octoscope")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(emptyDataJSON), nil
	}
	if _, err := c.GetPR(t.Context(), "herdr/herdr", 7); err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if repoVarsArg(got) != "herdr/herdr" {
		t.Errorf("args = %v, want the per-call repo to win", got)
	}
}

// RepoName takes no repository of its own, so the only thing that can name
// one is the client's. A call that asked about the working directory instead
// would answer for whichever repository octoscope happens to run in.
func TestRepoNameNamesTheClientsRepository(t *testing.T) {
	var got []string
	c := New("/tmp", "kukv/octoscope")
	c.run = func(_ context.Context, _ string, args ...string) ([]byte, error) {
		got = args
		return []byte(`{"data":{"repository":{"nameWithOwner":"kukv/octoscope"}}}`), nil
	}
	name, err := c.RepoName(t.Context())
	if err != nil {
		t.Fatalf("RepoName: %v", err)
	}
	if name != "kukv/octoscope" {
		t.Errorf("name = %q, want kukv/octoscope", name)
	}
	if repoVarsArg(got) != "kukv/octoscope" {
		t.Errorf("args = %v, want them to name the client's repository", got)
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

// TestTheLabelListAsksForMoreThanTheDefaultThirty states the requirement
// rather than the arguments: gh label list stops at 30 labels unless it is
// told otherwise, and it says nothing when it does. A repository with more
// labels than that would offer a picker missing the rest.
func TestTheLabelListAsksForMoreThanTheDefaultThirty(t *testing.T) {
	const ghDefaultLimit = 30

	c, f := newTestClient(`[]`, nil)
	if _, err := c.ListLabels(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	i := slices.Index(f.args, "--limit")
	if i < 0 || i+1 >= len(f.args) {
		t.Fatalf("args %v carry no --limit, so gh stops at %d", f.args, ghDefaultLimit)
	}
	limit, err := strconv.Atoi(f.args[i+1])
	if err != nil {
		t.Fatalf("--limit %q is not a number", f.args[i+1])
	}
	if limit <= ghDefaultLimit {
		t.Errorf("--limit %d asks for no more than the default %d", limit, ghDefaultLimit)
	}
}

// gh reports GitHub's front end giving up as an HTTP status on stderr; the
// body is nginx's HTML, not a GraphQL error. Callers retry this and nothing
// else, so it has to be told apart from a query GitHub actually answered.
func TestFrontEndFailuresAreTransient(t *testing.T) {
	for _, status := range []string{"502", "503", "504"} {
		t.Run(status, func(t *testing.T) {
			err := classify(fmt.Errorf("gh api: gh: HTTP %s", status))
			if !errors.Is(err, gh.ErrTransient) {
				t.Errorf("HTTP %s did not classify as transient: %v", status, err)
			}
		})
	}
}

func TestAnsweredFailuresAreNotTransient(t *testing.T) {
	for _, msg := range []string{
		"gh api: gh: Not Found (HTTP 404)",
		"gh api: gh: API rate limit exceeded",
		"gh pr list: no pull requests match",
	} {
		if err := classify(errors.New(msg)); errors.Is(err, gh.ErrTransient) {
			t.Errorf("%q classified as transient", msg)
		}
	}
}

// gh tells an expired or invalid token ("Bad credentials", HTTP 401) apart
// from having none at all ("gh auth login" in its own stderr); both need the
// same fix from the user, so both classify the same way.
func TestMissingCredentialsAreTold(t *testing.T) {
	for _, msg := range []string{
		"gh api: gh: Bad credentials (HTTP 401)",
		"gh api: To get started with GitHub CLI, please run:  gh auth login",
	} {
		if err := classify(errors.New(msg)); !errors.Is(err, gh.ErrUnauthenticated) {
			t.Errorf("%q did not classify as unauthenticated: %v", msg, err)
		}
	}
}

// The original text is what GitHub said, and the UI shows it as it was said
// (.claude/rules/errors.md). Classifying must not replace it, and must not
// add to it either: a sentinel's own words are a sentence octoscope wrote in
// English, and the notice that carries this text is shown in the user's own
// language. The classification travels by errors.Is, not by the text.
func TestClassifyLeavesTheTextExactlyAsGhSaidIt(t *testing.T) {
	for _, said := range []string{
		"gh api: gh: HTTP 502: Bad gateway",
		"gh pr list: gh: Bad credentials",
	} {
		err := classify(errors.New(said))
		if err.Error() != said {
			t.Errorf("classify rewrote the text:\n got %q\nwant %q", err.Error(), said)
		}
	}
}

// A read is safe to ask for twice. A transient failure is exactly the case
// where asking again is likely to work, so the caller never sees the first one.
func TestAReadIsAskedAgainAfterATransientFailure(t *testing.T) {
	c := New("", "kukv/demo")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
		}
		return []byte(emptyDataJSON), nil
	}
	if _, err := c.ListPRs(context.Background(), ""); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if calls != 2 {
		t.Errorf("gh ran %d times, want 2", calls)
	}
}

func TestAReadIsAskedAgainOnlyOnce(t *testing.T) {
	c := New("", "kukv/demo")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
	}
	if _, err := c.ListPRs(context.Background(), ""); err == nil {
		t.Fatal("ListPRs succeeded on a failing gh")
	}
	if calls != 2 {
		t.Errorf("gh ran %d times, want 2", calls)
	}
}

// A failure GitHub answered will answer the same way again.
func TestAnAnsweredFailureIsNotAskedAgain(t *testing.T) {
	c := New("", "kukv/demo")
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		return nil, errors.New("gh pr list: gh: Not Found (HTTP 404)")
	}
	_, _ = c.ListPRs(context.Background(), "")
	if calls != 1 {
		t.Errorf("gh ran %d times, want 1", calls)
	}
}

// A cancelled fetch must not be asked again: the user left, refreshed, or quit.
func TestACancelledReadIsNotAskedAgain(t *testing.T) {
	c := New("", "kukv/demo")
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		cancel()
		return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
	}
	_, _ = c.ListPRs(ctx, "")
	if calls != 1 {
		t.Errorf("gh ran %d times, want 1", calls)
	}
}

// 502 means "no answer came back", not "it never arrived": GitHub may have
// applied the change. Asking again could apply it twice.
func TestWritesAreNeverAskedAgain(t *testing.T) {
	writes := map[string]func(*Client) error{
		"AddPRComment":       func(c *Client) error { return c.AddPRComment("kukv/demo", 1, "hi") },
		"AddIssueComment":    func(c *Client) error { return c.AddIssueComment("kukv/demo", 1, "hi") },
		"ClosePR":            func(c *Client) error { return c.ClosePR("kukv/demo", 1) },
		"ReopenPR":           func(c *Client) error { return c.ReopenPR("kukv/demo", 1) },
		"CloseIssue":         func(c *Client) error { return c.CloseIssue("kukv/demo", 1) },
		"ReopenIssue":        func(c *Client) error { return c.ReopenIssue("kukv/demo", 1) },
		"EditPRLabels":       func(c *Client) error { return c.EditPRLabels("kukv/demo", 1, []string{"bug"}, nil) },
		"EditIssueLabels":    func(c *Client) error { return c.EditIssueLabels("kukv/demo", 1, []string{"bug"}, nil) },
		"EditPRAssignees":    func(c *Client) error { return c.EditPRAssignees("kukv/demo", 1, []string{"kukv"}, nil) },
		"EditIssueAssignees": func(c *Client) error { return c.EditIssueAssignees("kukv/demo", 1, []string{"kukv"}, nil) },
		"MergePR":            func(c *Client) error { return c.MergePR("id", gh.MergeSquash) },
		"EnableAutoMerge":    func(c *Client) error { return c.EnableAutoMerge("id", gh.MergeSquash) },
		"DisableAutoMerge":   func(c *Client) error { return c.DisableAutoMerge("id") },
		"AddReviewThread":    func(c *Client) error { return c.AddReviewThread("id", gh.PendingComment{}) },
		"SubmitReview":       func(c *Client) error { return c.SubmitReview("id", gh.EventApprove, "") },
		"SubmitNewReview":    func(c *Client) error { return c.SubmitNewReview("id", gh.EventApprove, "") },
		"DiscardReview":      func(c *Client) error { return c.DiscardReview("id") },
		"RerunWorkflow": func(c *Client) error {
			return c.RerunWorkflow(context.Background(), "kukv/demo", int64(1), gh.RerunFailed)
		},
	}
	for name, call := range writes {
		t.Run(name, func(t *testing.T) {
			c := New("", "kukv/demo")
			calls := 0
			c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
				calls++
				return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
			}
			_ = call(c)
			if calls != 1 {
				t.Errorf("%s ran gh %d times, want 1: a write must never be retried", name, calls)
			}
		})
	}
	t.Run("StartReview", func(t *testing.T) {
		c := New("", "kukv/demo")
		calls := 0
		c.run = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			return nil, fmt.Errorf("%w: gh: HTTP 502", gh.ErrTransient)
		}
		_, _ = c.StartReview("id")
		if calls != 1 {
			t.Errorf("StartReview ran gh %d times, want 1: a write must never be retried", calls)
		}
	})
}

func TestRepoVarsNamesTheOwnerAndName(t *testing.T) {
	t.Parallel()

	got, err := repoVars("kukv/koto")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	want := []gql.Var{gql.S("owner", "kukv"), gql.S("name", "koto")}
	if !slices.Equal(got, want) {
		t.Errorf("repoVars = %+v, want %+v", got, want)
	}
}

// TestRepoVarsFillsPlaceholdersWhenEmpty is the ordinary case: no --repo, so
// there is no "owner/name" to split and gh has to fill the placeholders from
// the checkout's remote.
func TestRepoVarsFillsPlaceholdersWhenEmpty(t *testing.T) {
	t.Parallel()

	got, err := repoVars("")
	if err != nil {
		t.Fatalf("repoVars: %v", err)
	}
	want := []gql.Var{gql.Placeholder("owner", "{owner}"), gql.Placeholder("name", "{repo}")}
	if !slices.Equal(got, want) {
		t.Errorf("repoVars = %+v, want %+v", got, want)
	}
}

// TestRepoVarsRejectsARepoWithNoSlash guards against silently querying the
// wrong repository: a --repo value with no "/" cannot be split into owner
// and name, so the call must fail rather than send an empty owner or name to
// GitHub.
func TestRepoVarsRejectsARepoWithNoSlash(t *testing.T) {
	t.Parallel()

	if _, err := repoVars("not-a-repo"); err == nil {
		t.Fatal("repoVars did not fail for a repo with no slash")
	}
}

// GraphQL rejects "3" where it wants 3, and gh substitutes {owner}/{repo}
// only in -F values. Everything the user typed stays in -f, where gh passes
// it through untouched.
func TestNumbersAndPlaceholdersAreTheOnlyTypedArguments(t *testing.T) {
	t.Parallel()

	got := ghArgs("query {}", []gql.Var{
		gql.Placeholder("owner", "{owner}"),
		gql.N("number", 3),
		gql.S("body", "-F not a flag"),
	})
	want := []string{
		"api", "graphql", "-f", "query=query {}",
		"-F", "owner={owner}",
		"-F", "number=3",
		"-f", "body=-F not a flag",
	}
	if !slices.Equal(got, want) {
		t.Errorf("args =\n%q\nwant\n%q", got, want)
	}
}

// --repo reaches the backends unvalidated, and the same string is handed to
// whichever one is running. This compares cli's parse against
// gql.SplitRepoVars -- the function the api backend parses with, not the api
// backend itself -- which is the strict one on purpose: the repository may
// also come from a hand-edited settings file. A raw strings.Cut here
// accepted "a/b/c", a half-empty name and a padded one, and asked GitHub for
// a repository nobody named.
func TestTheRepoStringIsSplitTheSameWayTheApiBackendSplitsIt(t *testing.T) {
	t.Parallel()

	for _, repo := range []string{"a/b/c", "kukv/", "/octoscope", " kukv/octoscope ", "octoscope"} {
		t.Run(repo, func(t *testing.T) {
			t.Parallel()

			if _, err := gql.SplitRepoVars(repo); err == nil {
				t.Fatalf("the api backend accepts %q, so this case proves nothing", repo)
			}
			if got, err := repoVars(repo); err == nil {
				t.Errorf("the cli backend turned %q into %+v; the api backend refuses it", repo, got)
			}
		})
	}
}
