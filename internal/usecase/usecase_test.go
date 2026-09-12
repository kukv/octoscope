package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/config"
	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	pr     gh.PR
	issue  gh.Issue
	err    error
	called []string

	checks       gh.Checks
	checksRepo   string
	checksNumber int

	logLines  []gh.LogLine
	logRepo   string
	logJobID  int64
	logFailed bool

	rerunRepo  string
	rerunRunID int64
	rerunScope gh.RerunScope

	mergeContext        gh.MergeContext
	mergedID            string
	mergedMethod        gh.MergeMethod
	autoMergeID         string
	autoMergeMethod     gh.MergeMethod
	disabledAutoMergeID string
}

func (f *fakeSource) GetPR(_ context.Context, _ string, _ int) (gh.PR, error) {
	f.called = append(f.called, "GetPR")
	return f.pr, f.err
}

func (f *fakeSource) GetIssue(_ context.Context, _ string, _ int) (gh.Issue, error) {
	f.called = append(f.called, "GetIssue")
	return f.issue, f.err
}

func (f *fakeSource) PRChecks(_ context.Context, repo string, number int) (gh.Checks, error) {
	f.checksRepo, f.checksNumber = repo, number
	return f.checks, f.err
}

func (f *fakeSource) JobLog(_ context.Context, repo string, jobID int64, failedOnly bool) ([]gh.LogLine, error) {
	f.logRepo, f.logJobID, f.logFailed = repo, jobID, failedOnly
	return f.logLines, f.err
}

func (f *fakeSource) RerunWorkflow(_ context.Context, repo string, runID int64, scope gh.RerunScope) error {
	f.rerunRepo, f.rerunRunID, f.rerunScope = repo, runID, scope
	return f.err
}

func (f *fakeSource) PRMergeContext(_ context.Context, _ string, _ int) (gh.MergeContext, error) {
	return f.mergeContext, f.err
}

func (f *fakeSource) MergePR(pullRequestID string, method gh.MergeMethod) error {
	f.mergedID, f.mergedMethod = pullRequestID, method
	return f.err
}

func (f *fakeSource) EnableAutoMerge(pullRequestID string, method gh.MergeMethod) error {
	f.autoMergeID, f.autoMergeMethod = pullRequestID, method
	return f.err
}

func (f *fakeSource) DisableAutoMerge(pullRequestID string) error {
	f.disabledAutoMergeID = pullRequestID
	return f.err
}

func TestGetItemFetchesAPullRequestForAPRRef(t *testing.T) {
	t.Parallel()

	f := &fakeSource{pr: gh.PR{
		Number: 55, Title: "feat: x", State: gh.StateOpen,
		Body: "body", URL: "https://example.test/pull/55",
		UpdatedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	}}
	u := &Usecase{items: f}

	item, err := u.GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemPR, Number: 55})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if len(f.called) != 1 || f.called[0] != "GetPR" {
		t.Fatalf("calls = %v, want exactly [GetPR]", f.called)
	}
	if item.Kind != gh.ItemPR || item.Number != 55 || item.Title != "feat: x" {
		t.Errorf("item = %+v, want the pull request's own fields", item)
	}
	if item.URL != "https://example.test/pull/55" {
		t.Errorf("url = %q; o would have nothing to open", item.URL)
	}
	if item.PR == nil {
		t.Fatal("PR is nil on a pull request; the detail view loses head/base and checks")
	}
	if item.PR.Number != 55 {
		t.Errorf("PR.Number = %d, want 55", item.PR.Number)
	}
}

func TestGetItemFetchesAnIssueForAnIssueRef(t *testing.T) {
	t.Parallel()

	f := &fakeSource{issue: gh.Issue{
		Number: 50, Title: "bug: y", State: gh.StateClosed,
		Body: "body", URL: "https://example.test/issues/50",
	}}
	u := &Usecase{items: f}

	item, err := u.GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemIssue, Number: 50})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if len(f.called) != 1 || f.called[0] != "GetIssue" {
		t.Fatalf("calls = %v, want exactly [GetIssue]", f.called)
	}
	if item.Kind != gh.ItemIssue || item.Number != 50 || item.State != gh.StateClosed {
		t.Errorf("item = %+v, want the issue's own fields", item)
	}
	if item.PR != nil {
		t.Error("PR is set on an issue; there is no pull request behind it")
	}
}

func TestGetItemPassesTheFetchFailureThrough(t *testing.T) {
	t.Parallel()

	want := errors.New("boom")
	u := &Usecase{items: &fakeSource{err: want}}

	if _, err := u.GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemPR}); !errors.Is(err, want) {
		t.Errorf("err = %v, want it to wrap %v", err, want)
	}
}

// .claude/rules/architecture.md requires Item to carry every field common to
// gh.PR and gh.Issue; a copy that silently drops one is what this catches.
func TestGetItemCopiesEveryFieldAGhIssueHas(t *testing.T) {
	t.Parallel()

	updated := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	labels := []gh.Label{{Name: "bug", Color: "ff0000"}}
	assignees := []gh.Author{{Login: "reviewer"}}
	comments := []gh.Comment{{Author: gh.Author{Login: "commenter"}, Body: "hi"}}

	pr := gh.PR{
		Number: 55, Title: "feat: x", Author: gh.Author{Login: "author"},
		State: gh.StateOpen, Body: "pr body", URL: "https://example.test/pull/55",
		Labels: labels, Assignees: assignees, Comments: comments, UpdatedAt: updated,
	}
	item, err := (&Usecase{items: &fakeSource{pr: pr}}).GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemPR})
	if err != nil {
		t.Fatalf("GetItem(PR): %v", err)
	}
	assertItemMatchesCommonFields(t, "PR", item, pr.Number, pr.Title, pr.Author, pr.State, pr.Body, pr.URL, pr.Labels, pr.Assignees, pr.Comments, pr.UpdatedAt)
	if item.PR == nil {
		t.Error("PR is nil on a pull request")
	}

	issue := gh.Issue{
		Number: 50, Title: "bug: y", Author: gh.Author{Login: "author2"},
		State: gh.StateClosed, Body: "issue body", URL: "https://example.test/issues/50",
		Labels: labels, Assignees: assignees, Comments: comments, UpdatedAt: updated,
	}
	item, err = (&Usecase{items: &fakeSource{issue: issue}}).GetItem(t.Context(), gh.ItemRef{Kind: gh.ItemIssue})
	if err != nil {
		t.Fatalf("GetItem(Issue): %v", err)
	}
	assertItemMatchesCommonFields(t, "Issue", item, issue.Number, issue.Title, issue.Author, issue.State, issue.Body, issue.URL, issue.Labels, issue.Assignees, issue.Comments, issue.UpdatedAt)
	if item.PR != nil {
		t.Error("PR is set on an issue")
	}
}

func assertItemMatchesCommonFields(
	t *testing.T, kind string, item Item,
	number int, title string, author gh.Author, state gh.ItemState, body, url string,
	labels []gh.Label, assignees []gh.Author, comments []gh.Comment, updatedAt time.Time,
) {
	t.Helper()
	if item.Number != number {
		t.Errorf("%s: Number = %d, want %d", kind, item.Number, number)
	}
	if item.Title != title {
		t.Errorf("%s: Title = %q, want %q", kind, item.Title, title)
	}
	if item.Author != author {
		t.Errorf("%s: Author = %+v, want %+v", kind, item.Author, author)
	}
	if item.State != state {
		t.Errorf("%s: State = %v, want %v", kind, item.State, state)
	}
	if item.Body != body {
		t.Errorf("%s: Body = %q, want %q", kind, item.Body, body)
	}
	if item.URL != url {
		t.Errorf("%s: URL = %q, want %q", kind, item.URL, url)
	}
	if !slices.Equal(item.Labels, labels) {
		t.Errorf("%s: Labels = %+v, want %+v", kind, item.Labels, labels)
	}
	if !slices.Equal(item.Assignees, assignees) {
		t.Errorf("%s: Assignees = %+v, want %+v", kind, item.Assignees, assignees)
	}
	if !slices.Equal(item.Comments, comments) {
		t.Errorf("%s: Comments = %+v, want %+v", kind, item.Comments, comments)
	}
	if !item.UpdatedAt.Equal(updatedAt) {
		t.Errorf("%s: UpdatedAt = %v, want %v", kind, item.UpdatedAt, updatedAt)
	}
}

type fakeWriter struct {
	calls []string
	err   error
}

func (f *fakeWriter) AddPRComment(_ string, _ int, _ string) error {
	f.calls = append(f.calls, "AddPRComment")
	return f.err
}

func (f *fakeWriter) AddIssueComment(_ string, _ int, _ string) error {
	f.calls = append(f.calls, "AddIssueComment")
	return f.err
}

func (f *fakeWriter) ClosePR(_ string, _ int) error {
	f.calls = append(f.calls, "ClosePR")
	return f.err
}

func (f *fakeWriter) ReopenPR(_ string, _ int) error {
	f.calls = append(f.calls, "ReopenPR")
	return f.err
}

func (f *fakeWriter) CloseIssue(_ string, _ int) error {
	f.calls = append(f.calls, "CloseIssue")
	return f.err
}

func (f *fakeWriter) ReopenIssue(_ string, _ int) error {
	f.calls = append(f.calls, "ReopenIssue")
	return f.err
}

func (f *fakeWriter) EditPRLabels(_ string, _ int, _, _ []string) error {
	f.calls = append(f.calls, "EditPRLabels")
	return f.err
}

func (f *fakeWriter) EditIssueLabels(_ string, _ int, _, _ []string) error {
	f.calls = append(f.calls, "EditIssueLabels")
	return f.err
}

func (f *fakeWriter) EditPRAssignees(_ string, _ int, _, _ []string) error {
	f.calls = append(f.calls, "EditPRAssignees")
	return f.err
}

func (f *fakeWriter) EditIssueAssignees(_ string, _ int, _, _ []string) error {
	f.calls = append(f.calls, "EditIssueAssignees")
	return f.err
}

func TestAddCommentPicksTheCallByKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind gh.ItemKind
		want string
	}{
		{gh.ItemPR, "AddPRComment"},
		{gh.ItemIssue, "AddIssueComment"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{comments: f}
		if err := u.AddComment(gh.ItemRef{Kind: tc.kind}, "hi"); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if !slices.Equal(f.calls, []string{tc.want}) {
			t.Errorf("kind %v: calls = %v, want [%s]", tc.kind, f.calls, tc.want)
		}
	}
}

func TestSetStatePicksTheCallByKindAndDirection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		kind    gh.ItemKind
		closing bool
		want    string
	}{
		{"PR closing", gh.ItemPR, true, "ClosePR"},
		{"PR reopening", gh.ItemPR, false, "ReopenPR"},
		{"Issue closing", gh.ItemIssue, true, "CloseIssue"},
		{"Issue reopening", gh.ItemIssue, false, "ReopenIssue"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{states: f}
		if err := u.SetState(gh.ItemRef{Kind: tc.kind}, tc.closing); err != nil {
			t.Fatalf("%s: SetState: %v", tc.name, err)
		}
		if !slices.Equal(f.calls, []string{tc.want}) {
			t.Errorf("%s: calls = %v, want [%s]", tc.name, f.calls, tc.want)
		}
	}
}

func TestEditLabelsPicksTheCallByKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind gh.ItemKind
		want string
	}{
		{gh.ItemPR, "EditPRLabels"},
		{gh.ItemIssue, "EditIssueLabels"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{labels: f}
		if err := u.EditLabels(gh.ItemRef{Kind: tc.kind}, []string{"a"}, []string{"b"}); err != nil {
			t.Fatalf("EditLabels: %v", err)
		}
		if !slices.Equal(f.calls, []string{tc.want}) {
			t.Errorf("kind %v: calls = %v, want [%s]", tc.kind, f.calls, tc.want)
		}
	}
}

func TestMergePRPassesTheMethodThrough(t *testing.T) {
	t.Parallel()

	f := &fakeSource{}
	u := &Usecase{merges: f}
	if err := u.MergePR("PR_1", gh.MergeRebase); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if f.mergedID != "PR_1" || f.mergedMethod != gh.MergeRebase {
		t.Errorf("merged (%q, %v), want (%q, %v)", f.mergedID, f.mergedMethod, "PR_1", gh.MergeRebase)
	}
}

func TestPRChecksReachesTheBackend(t *testing.T) {
	t.Parallel()

	f := &fakeSource{checks: gh.Checks{Total: 3}}
	u := &Usecase{checks: f}

	got, err := u.PRChecks(t.Context(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	if got.Total != 3 {
		t.Errorf("Total = %d, want 3", got.Total)
	}
	if f.checksRepo != "kukv/octoscope" || f.checksNumber != 61 {
		t.Errorf("backend was asked for %s#%d, want kukv/octoscope#61", f.checksRepo, f.checksNumber)
	}
}

func TestJobLogReachesTheBackend(t *testing.T) {
	t.Parallel()

	f := &fakeSource{logLines: []gh.LogLine{{Text: "hi"}}}
	u := &Usecase{checks: f}

	got, err := u.JobLog(t.Context(), "kukv/octoscope", 61, true)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if !slices.Equal(got, f.logLines) {
		t.Errorf("JobLog = %+v, want %+v", got, f.logLines)
	}
	if f.logRepo != "kukv/octoscope" || f.logJobID != 61 || !f.logFailed {
		t.Errorf("backend was asked for %s job %d failedOnly=%v, want kukv/octoscope job 61 failedOnly=true",
			f.logRepo, f.logJobID, f.logFailed)
	}
}

func TestRerunWorkflowReachesTheBackend(t *testing.T) {
	t.Parallel()

	f := &fakeSource{}
	u := &Usecase{checks: f}

	if err := u.RerunWorkflow(t.Context(), "kukv/octoscope", 61, gh.RerunAll); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if f.rerunRepo != "kukv/octoscope" || f.rerunRunID != 61 || f.rerunScope != gh.RerunAll {
		t.Errorf("backend was asked to rerun %s run %d scope=%v, want kukv/octoscope run 61 scope=RerunAll",
			f.rerunRepo, f.rerunRunID, f.rerunScope)
	}
}

// fakeCrossRepo records the query SearchItems was asked to run.
type fakeCrossRepo struct {
	searchQuery string
	err         error
}

func (f *fakeCrossRepo) ListWorkSection(_ context.Context, _ gh.WorkSection) ([]gh.WorkItem, error) {
	return nil, f.err
}

func (f *fakeCrossRepo) RepoCounts(_ context.Context, _ []string) ([]gh.RepoCount, error) {
	return nil, f.err
}

func (f *fakeCrossRepo) SearchItems(_ context.Context, query string) ([]gh.WorkItem, error) {
	f.searchQuery = query
	return nil, f.err
}

func TestSearchItemsReachesTheGitHubLayer(t *testing.T) {
	t.Parallel()

	f := &fakeCrossRepo{}
	u := &Usecase{crossRepo: f}
	if _, err := u.SearchItems(t.Context(), "is:open is:pr"); err != nil {
		t.Fatalf("SearchItems: %v", err)
	}
	if f.searchQuery != "is:open is:pr" {
		t.Errorf("the query reached the layer as %q", f.searchQuery)
	}
}

type fakeQueryStore struct {
	saved []config.SavedQuery
	err   error
}

func (f *fakeQueryStore) SaveQueries(queries []config.SavedQuery) error {
	f.saved = queries
	return f.err
}

// SaveQueries converts usecase.SavedQuery into config.SavedQuery on the way
// to the store: internal/config is not visible from internal/tui, so the
// UI's type cannot be the one written to disk.
func TestSaveQueriesConvertsAndReachesTheStore(t *testing.T) {
	t.Parallel()

	store := &fakeQueryStore{}
	u := &Usecase{queryStore: store}
	if err := u.SaveQueries([]SavedQuery{{Name: "mine", Query: "is:open author:@me"}}); err != nil {
		t.Fatalf("SaveQueries: %v", err)
	}
	want := []config.SavedQuery{{Name: "mine", Query: "is:open author:@me"}}
	if !slices.Equal(store.saved, want) {
		t.Errorf("store holds %+v, want %+v", store.saved, want)
	}
}

// cmd/octoscope reads config.SavedQuery from the settings file and must
// hand app.Options the UI's own type instead.
func TestSavedQueriesFromConvertsForStartup(t *testing.T) {
	t.Parallel()

	got := SavedQueriesFrom([]config.SavedQuery{{Name: "mine", Query: "is:open"}})
	want := []SavedQuery{{Name: "mine", Query: "is:open"}}
	if !slices.Equal(got, want) {
		t.Errorf("SavedQueriesFrom = %+v, want %+v", got, want)
	}
}

func TestEditAssigneesPicksTheCallByKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind gh.ItemKind
		want string
	}{
		{gh.ItemPR, "EditPRAssignees"},
		{gh.ItemIssue, "EditIssueAssignees"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{assignees: f}
		if err := u.EditAssignees(gh.ItemRef{Kind: tc.kind}, []string{"a"}, []string{"b"}); err != nil {
			t.Fatalf("EditAssignees: %v", err)
		}
		if !slices.Equal(f.calls, []string{tc.want}) {
			t.Errorf("kind %v: calls = %v, want [%s]", tc.kind, f.calls, tc.want)
		}
	}
}
