package gh

import (
	"context"
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// wrap is the only place a client's sentinel becomes the domain's; every
// override in this package depends on it to keep the fatal-error screen
// working, so each sentinel it knows about is checked here, plus the case it
// must never touch: an error none of them name.
func TestWrapTranslatesEverySentinel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   error
		want error
	}{
		{"not installed", github.ErrNotInstalled, domain.ErrBackendUnavailable},
		{"unauthenticated", github.Classify(github.ErrUnauthenticated, "gh: Bad credentials"), domain.ErrUnauthenticated},
		{"transient", github.Classify(github.ErrTransient, "gh: HTTP 502"), domain.ErrTransient},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrap(tt.in)
			if !errors.Is(got, tt.want) {
				t.Errorf("wrap(%v) = %v, want errors.Is(_, %v)", tt.in, got, tt.want)
			}
			if got.Error() != tt.in.Error() {
				t.Errorf("wrap rewrote the text:\n got %q\nwant %q", got.Error(), tt.in.Error())
			}
		})
	}
}

// A sentinel wrap does not recognise must reach the caller unchanged: this is
// the one seam every override's error crosses, and swallowing an unknown
// error here would hide it from all of them.
func TestWrapLeavesAnUnknownErrorUnchanged(t *testing.T) {
	t.Parallel()

	in := errors.New("gh pr list: no pull requests match")
	got := wrap(in)
	if got != in {
		t.Errorf("wrap(%v) = %v, want the same error unchanged", in, got)
	}
	if errors.Is(got, domain.ErrBackendUnavailable) || errors.Is(got, domain.ErrUnauthenticated) {
		t.Errorf("wrap(%v) = %v, want it not to be fatal", in, got)
	}
}

func TestWrapPassesNilThrough(t *testing.T) {
	t.Parallel()

	if err := wrap(nil); err != nil {
		t.Errorf("wrap(nil) = %v, want nil", err)
	}
}

func (f fakeBackend) PRChecks(ctx context.Context, repo string, number int) ([]gql.CheckRun, error) {
	return f.prChecks(ctx, repo, number)
}

func (f fakeBackend) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]github.LogLine, error) {
	return f.jobLog(ctx, repo, jobID, failedOnly)
}

func (f fakeBackend) RerunWorkflow(ctx context.Context, repo string, runID int64, scope github.RerunScope) error {
	return f.rerunWorkflow(ctx, repo, runID, scope)
}

func (f fakeBackend) ListPRs(ctx context.Context, repo string) ([]gql.PullRequest, error) {
	return f.listPRs(ctx, repo)
}

func (f fakeBackend) ListIssues(ctx context.Context, repo string) ([]gql.Issue, error) {
	return f.listIssues(ctx, repo)
}

func (f fakeBackend) ListLabels(ctx context.Context, repo string) ([]gql.Label, error) {
	return f.listLabels(ctx, repo)
}

func (f fakeBackend) PRMergeContext(ctx context.Context, repo string, number int) (gql.MergeContext, error) {
	return f.prMergeContext(ctx, repo, number)
}

func (f fakeBackend) MergePR(pullRequestID string, method gql.MergeMethod) error {
	return f.mergePR(pullRequestID, method)
}

func (f fakeBackend) EnableAutoMerge(pullRequestID string, method gql.MergeMethod) error {
	return f.enableAutoMerge(pullRequestID, method)
}

func (f fakeBackend) SearchRepos(ctx context.Context, query string, limit int) ([]github.Repository, error) {
	return f.searchRepos(ctx, query, limit)
}

func (f fakeBackend) ListOwnRepos(ctx context.Context, owner string, limit int) ([]github.Repository, error) {
	return f.listOwnRepos(ctx, owner, limit)
}

func (f fakeBackend) SearchItems(ctx context.Context, query string) ([]gql.SearchItem, error) {
	return f.searchItems(ctx, query)
}

func (f fakeBackend) RepoCounts(ctx context.Context, repos []string) ([]gql.RepoCount, error) {
	return f.repoCounts(ctx, repos)
}

// TestEveryOverrideKeepsFatalErrorsFatal drives every one of the gateway's
// 21 overrides through a backend that answers the way cli.Client's classify
// does, to the exact check the fatal-error screen makes. A missed wrap on
// any single override would pass this same shape of error through
// unclassified, and domain.IsFatal would stop seeing it for that one method
// -- which is what leaves the user looking at an empty screen instead of one
// telling them to sign in.
func TestEveryOverrideKeepsFatalErrorsFatal(t *testing.T) {
	t.Parallel()

	authErr := github.Classify(github.ErrUnauthenticated, "gh: Bad credentials")

	g := New(fakeBackend{
		getPR:           func(context.Context, string, int) (gql.PullRequest, error) { return gql.PullRequest{}, authErr },
		getIssue:        func(context.Context, string, int) (gql.Issue, error) { return gql.Issue{}, authErr },
		prDiff:          func(context.Context, string, int) (github.Diff, error) { return github.Diff{}, authErr },
		prReviewContext: func(context.Context, string, int) (gql.ReviewContext, error) { return gql.ReviewContext{}, authErr },
		addReviewThread: func(string, gql.PendingComment) error { return authErr },
		submitReview:    func(string, gql.ReviewEvent, string) error { return authErr },
		submitNewReview: func(string, gql.ReviewEvent, string) error { return authErr },
		prChecks:        func(context.Context, string, int) ([]gql.CheckRun, error) { return nil, authErr },
		jobLog:          func(context.Context, string, int64, bool) ([]github.LogLine, error) { return nil, authErr },
		rerunWorkflow:   func(context.Context, string, int64, github.RerunScope) error { return authErr },
		listPRs:         func(context.Context, string) ([]gql.PullRequest, error) { return nil, authErr },
		listIssues:      func(context.Context, string) ([]gql.Issue, error) { return nil, authErr },
		listLabels:      func(context.Context, string) ([]gql.Label, error) { return nil, authErr },
		prMergeContext:  func(context.Context, string, int) (gql.MergeContext, error) { return gql.MergeContext{}, authErr },
		mergePR:         func(string, gql.MergeMethod) error { return authErr },
		enableAutoMerge: func(string, gql.MergeMethod) error { return authErr },
		searchRepos:     func(context.Context, string, int) ([]github.Repository, error) { return nil, authErr },
		listOwnRepos:    func(context.Context, string, int) ([]github.Repository, error) { return nil, authErr },
		searchItems:     func(context.Context, string) ([]gql.SearchItem, error) { return nil, authErr },
		repoCounts:      func(context.Context, []string) ([]gql.RepoCount, error) { return nil, authErr },
	})

	tests := []struct {
		name string
		call func() error
	}{
		{"PRChecks", func() error { _, err := g.PRChecks(context.Background(), "kukv/octoscope", 1); return err }},
		{"JobLog", func() error { _, err := g.JobLog(context.Background(), "kukv/octoscope", 1, false); return err }},
		{"RerunWorkflow", func() error { return g.RerunWorkflow(context.Background(), "kukv/octoscope", 1, domain.RerunAll) }},
		{"GetPR", func() error { _, err := g.GetPR(context.Background(), "kukv/octoscope", 1); return err }},
		{"GetIssue", func() error { _, err := g.GetIssue(context.Background(), "kukv/octoscope", 1); return err }},
		{"ListPRs", func() error { _, err := g.ListPRs(context.Background(), "kukv/octoscope"); return err }},
		{"ListIssues", func() error { _, err := g.ListIssues(context.Background(), "kukv/octoscope"); return err }},
		{"ListLabels", func() error { _, err := g.ListLabels(context.Background(), "kukv/octoscope"); return err }},
		{"PRMergeContext", func() error { _, err := g.PRMergeContext(context.Background(), "kukv/octoscope", 1); return err }},
		{"MergePR", func() error { return g.MergePR("pr-id", domain.MergeSquash) }},
		{"EnableAutoMerge", func() error { return g.EnableAutoMerge("pr-id", domain.MergeSquash) }},
		{"SearchRepos", func() error { _, err := g.SearchRepos(context.Background(), "octoscope", 10); return err }},
		{"ListOwnRepos", func() error { _, err := g.ListOwnRepos(context.Background(), "kukv", 10); return err }},
		{"PRDiff", func() error { _, err := g.PRDiff(context.Background(), "kukv/octoscope", 1); return err }},
		{"PRReviewContext", func() error { _, err := g.PRReviewContext(context.Background(), "kukv/octoscope", 1); return err }},
		{"AddReviewThread", func() error { return g.AddReviewThread("review-id", domain.PendingComment{}) }},
		{"SubmitReview", func() error { return g.SubmitReview("review-id", domain.EventApprove, "") }},
		{"SubmitNewReview", func() error { return g.SubmitNewReview("pr-id", domain.EventApprove, "") }},
		{"ListWorkSection", func() error { _, err := g.ListWorkSection(context.Background(), domain.SectionYourPRs); return err }},
		{"SearchItems", func() error { _, err := g.SearchItems(context.Background(), "is:open"); return err }},
		{"RepoCounts", func() error { _, err := g.RepoCounts(context.Background(), []string{"kukv/octoscope"}); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !domain.IsFatal(err) {
				t.Errorf("%s() error = %v, want domain.IsFatal", tt.name, err)
			}
		})
	}
}
