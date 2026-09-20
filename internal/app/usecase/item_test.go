package usecase

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
)

func TestGetItemFetchesAPullRequestForAPRRef(t *testing.T) {
	t.Parallel()

	f := &fakeSource{pr: domain.PR{
		Number: 55, Title: "feat: x", State: domain.StateOpen,
		Body: "body", URL: "https://example.test/pull/55",
		UpdatedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
	}}
	u := &Usecase{items: f}

	item, err := u.GetItem(t.Context(), domain.ItemRef{Kind: domain.ItemPR, Number: 55})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if len(f.called) != 1 || f.called[0] != "GetPR" {
		t.Fatalf("calls = %v, want exactly [GetPR]", f.called)
	}
	if item.Kind != domain.ItemPR || item.Number != 55 || item.Title != "feat: x" {
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

	f := &fakeSource{issue: domain.Issue{
		Number: 50, Title: "bug: y", State: domain.StateClosed,
		Body: "body", URL: "https://example.test/issues/50",
	}}
	u := &Usecase{items: f}

	item, err := u.GetItem(t.Context(), domain.ItemRef{Kind: domain.ItemIssue, Number: 50})
	if err != nil {
		t.Fatalf("GetItem: %v", err)
	}
	if len(f.called) != 1 || f.called[0] != "GetIssue" {
		t.Fatalf("calls = %v, want exactly [GetIssue]", f.called)
	}
	if item.Kind != domain.ItemIssue || item.Number != 50 || item.State != domain.StateClosed {
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

	if _, err := u.GetItem(t.Context(), domain.ItemRef{Kind: domain.ItemPR}); !errors.Is(err, want) {
		t.Errorf("err = %v, want it to wrap %v", err, want)
	}
}

// .claude/rules/architecture.md requires Item to carry every field common to
// domain.PR and domain.Issue; a copy that silently drops one is what this catches.
func TestGetItemCopiesEveryFieldAGhIssueHas(t *testing.T) {
	t.Parallel()

	updated := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	labels := []domain.Label{{Name: "bug", Color: "ff0000"}}
	assignees := []domain.Author{{Login: "reviewer"}}
	comments := []domain.Comment{{Author: domain.Author{Login: "commenter"}, Body: "hi"}}

	pr := domain.PR{
		Number: 55, Title: "feat: x", Author: domain.Author{Login: "author"},
		State: domain.StateOpen, Body: "pr body", URL: "https://example.test/pull/55",
		Labels: labels, Assignees: assignees, Comments: comments, UpdatedAt: updated,
	}
	item, err := (&Usecase{items: &fakeSource{pr: pr}}).GetItem(t.Context(), domain.ItemRef{Kind: domain.ItemPR})
	if err != nil {
		t.Fatalf("GetItem(PR): %v", err)
	}
	assertItemMatchesCommonFields(t, "PR", item, pr.Number, pr.Title, pr.Author, pr.State, pr.Body, pr.URL, pr.Labels, pr.Assignees, pr.Comments, pr.UpdatedAt)
	if item.PR == nil {
		t.Error("PR is nil on a pull request")
	}

	issue := domain.Issue{
		Number: 50, Title: "bug: y", Author: domain.Author{Login: "author2"},
		State: domain.StateClosed, Body: "issue body", URL: "https://example.test/issues/50",
		Labels: labels, Assignees: assignees, Comments: comments, UpdatedAt: updated,
	}
	item, err = (&Usecase{items: &fakeSource{issue: issue}}).GetItem(t.Context(), domain.ItemRef{Kind: domain.ItemIssue})
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
	number int, title string, author domain.Author, state domain.ItemState, body, url string,
	labels []domain.Label, assignees []domain.Author, comments []domain.Comment, updatedAt time.Time,
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
		kind domain.ItemKind
		want string
	}{
		{domain.ItemPR, "AddPRComment"},
		{domain.ItemIssue, "AddIssueComment"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{comments: f}
		if err := u.AddComment(domain.ItemRef{Kind: tc.kind}, "hi"); err != nil {
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
		kind    domain.ItemKind
		closing bool
		want    string
	}{
		{"PR closing", domain.ItemPR, true, "ClosePR"},
		{"PR reopening", domain.ItemPR, false, "ReopenPR"},
		{"Issue closing", domain.ItemIssue, true, "CloseIssue"},
		{"Issue reopening", domain.ItemIssue, false, "ReopenIssue"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{states: f}
		if err := u.SetState(domain.ItemRef{Kind: tc.kind}, tc.closing); err != nil {
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
		kind domain.ItemKind
		want string
	}{
		{domain.ItemPR, "EditPRLabels"},
		{domain.ItemIssue, "EditIssueLabels"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{labels: f}
		if err := u.EditLabels(domain.ItemRef{Kind: tc.kind}, []string{"a"}, []string{"b"}); err != nil {
			t.Fatalf("EditLabels: %v", err)
		}
		if !slices.Equal(f.calls, []string{tc.want}) {
			t.Errorf("kind %v: calls = %v, want [%s]", tc.kind, f.calls, tc.want)
		}
	}
}

func TestEditAssigneesPicksTheCallByKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind domain.ItemKind
		want string
	}{
		{domain.ItemPR, "EditPRAssignees"},
		{domain.ItemIssue, "EditIssueAssignees"},
	}
	for _, tc := range tests {
		f := &fakeWriter{}
		u := &Usecase{assignees: f}
		if err := u.EditAssignees(domain.ItemRef{Kind: tc.kind}, []string{"a"}, []string{"b"}); err != nil {
			t.Fatalf("EditAssignees: %v", err)
		}
		if !slices.Equal(f.calls, []string{tc.want}) {
			t.Errorf("kind %v: calls = %v, want [%s]", tc.kind, f.calls, tc.want)
		}
	}
}
