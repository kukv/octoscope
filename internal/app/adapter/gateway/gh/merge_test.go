package gh

import (
	"context"
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github/gql"
)

// fakeMerger answers PRMergeContext, MergePR and EnableAutoMerge with
// whatever a test sets. Embedding the nil backend panics loudly if a test
// calls a method it did not mean to exercise.
type fakeMerger struct {
	backend
	prMergeContext  func(ctx context.Context, repo string, number int) (gql.MergeContext, error)
	mergePR         func(pullRequestID string, method gql.MergeMethod) error
	enableAutoMerge func(pullRequestID string, method gql.MergeMethod) error
}

func (f fakeMerger) PRMergeContext(ctx context.Context, repo string, number int) (gql.MergeContext, error) {
	return f.prMergeContext(ctx, repo, number)
}

func (f fakeMerger) MergePR(pullRequestID string, method gql.MergeMethod) error {
	return f.mergePR(pullRequestID, method)
}

func (f fakeMerger) EnableAutoMerge(pullRequestID string, method gql.MergeMethod) error {
	return f.enableAutoMerge(pullRequestID, method)
}

// TestToMergeContextTranslatesEveryField gives every field of
// gql.MergeContext a distinct, non-zero value and compares the whole
// resulting domain.MergeContext against a fully written-out expectation.
func TestToMergeContextTranslatesEveryField(t *testing.T) {
	t.Parallel()

	c := gql.MergeContext{
		PullRequestID:            "PR_kwDOTVXF-M8AAAABCd9eQA",
		IsDraft:                  true,
		Mergeable:                "CONFLICTING",
		MergeStateStatus:         "DIRTY",
		ReviewDecision:           "APPROVED",
		SquashMergeAllowed:       true,
		MergeCommitAllowed:       false,
		RebaseMergeAllowed:       true,
		DeleteBranchOnMerge:      true,
		AutoMergeAllowed:         true,
		ViewerCanEnableAutoMerge: true,
		AutoMergeEnabled:         true,
	}
	got := toMergeContext(c)
	want := domain.MergeContext{
		PullRequest:              "PR_kwDOTVXF-M8AAAABCd9eQA",
		Block:                    domain.BlockDraft,
		Review:                   domain.ReviewApproved,
		Methods:                  []domain.MergeMethod{domain.MergeSquash, domain.MergeRebase},
		DeleteBranchOnMerge:      true,
		AutoMergeAllowed:         true,
		ViewerCanEnableAutoMerge: true,
		AutoMergeEnabled:         true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toMergeContext() = %+v, want %+v", got, want)
	}
}

// TestToMergeBlockReadsWhatTheServiceReported is the table that used to live
// on domain.MergeContext.Block. It moved here with the translation it does:
// draft-first exists because GitHub reports a draft as BLOCKED, which is a
// fact about GitHub and not a rule of this application.
func TestToMergeBlockReadsWhatTheServiceReported(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		isDraft   bool
		mergeable string
		state     string
		want      domain.MergeBlock
	}{
		{"clean", false, "MERGEABLE", "CLEAN", domain.BlockNone},
		{"draft outranks the state GitHub reports for it", true, "MERGEABLE", "BLOCKED", domain.BlockDraft},
		{"conflicting", false, "CONFLICTING", "DIRTY", domain.BlockConflicting},
		{"still computing", false, "UNKNOWN", "UNKNOWN", domain.BlockComputing},
		{"protected", false, "MERGEABLE", "BLOCKED", domain.BlockProtected},
		{"behind", false, "MERGEABLE", "BEHIND", domain.BlockBehind},
		{"dirty", false, "MERGEABLE", "DIRTY", domain.BlockDirty},
		{"failing checks do not block: GitHub allows the merge", false, "MERGEABLE", "UNSTABLE", domain.BlockNone},
		{"hooks do not block either", false, "MERGEABLE", "HAS_HOOKS", domain.BlockNone},
		{"a mergeable word we do not know means the answer is not in yet", false, "NEW_WORD", "CLEAN", domain.BlockComputing},
		{"a state word we do not know refuses nothing", false, "MERGEABLE", "NEW_WORD", domain.BlockNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := toMergeBlock(tt.isDraft, tt.mergeable, tt.state)
			if got != tt.want {
				t.Errorf("toMergeBlock(%v, %q, %q) = %v, want %v",
					tt.isDraft, tt.mergeable, tt.state, got, tt.want)
			}
		})
	}
}

// Clean is not the same question as Block: UNSTABLE refuses nothing, but
// there is still something to wait for, and that is exactly when auto-merge
// is worth offering.
func TestCleanIsOnlyGitHubsCleanState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state string
		want  bool
	}{
		{"CLEAN", true},
		{"UNSTABLE", false},
		{"HAS_HOOKS", false},
		{"BLOCKED", false},
		{"UNKNOWN", false},
		{"NEW_WORD", false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()

			got := toMergeContext(gql.MergeContext{
				Mergeable: "MERGEABLE", MergeStateStatus: tt.state,
			}).Clean
			if got != tt.want {
				t.Errorf("Clean for %q = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

// TestAllowedMethodsOrdersThem lists the methods in the order the popup
// draws them regardless of which are on: squash, merge commit, rebase.
func TestAllowedMethodsOrdersThem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                   string
		squash, commit, rebase bool
		want                   []domain.MergeMethod
	}{
		{"all off", false, false, false, nil},
		{"all on", true, true, true, []domain.MergeMethod{domain.MergeSquash, domain.MergeCommit, domain.MergeRebase}},
		{"only rebase", false, false, true, []domain.MergeMethod{domain.MergeRebase}},
		{"squash and rebase", true, false, true, []domain.MergeMethod{domain.MergeSquash, domain.MergeRebase}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := allowedMethods(tt.squash, tt.commit, tt.rebase)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("allowedMethods(%v, %v, %v) = %v, want %v", tt.squash, tt.commit, tt.rebase, got, tt.want)
			}
		})
	}
}

// TestFromMergeMethodMapsEveryValue guards the only place that decides what
// PullRequestMergeMethod string a chosen method turns into. Squash and
// rebase are easy to swap by accident, and a wrong mapping merges the pull
// request the way the user did not choose without ever failing.
func TestFromMergeMethodMapsEveryValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method domain.MergeMethod
		want   gql.MergeMethod
	}{
		{domain.MergeSquash, gql.MergeMethodSquash},
		{domain.MergeCommit, gql.MergeMethodMerge},
		{domain.MergeRebase, gql.MergeMethodRebase},
	}
	for _, tt := range tests {
		t.Run(string(tt.want), func(t *testing.T) {
			t.Parallel()

			if got := fromMergeMethod(tt.method); got != tt.want {
				t.Errorf("fromMergeMethod(%v) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

// TestPRMergeContextTranslatesWhatTheBackendReturns wires PRMergeContext
// through the gateway end to end.
func TestPRMergeContextTranslatesWhatTheBackendReturns(t *testing.T) {
	t.Parallel()

	g := New(fakeMerger{prMergeContext: func(context.Context, string, int) (gql.MergeContext, error) {
		return gql.MergeContext{PullRequestID: "PR_1", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"}, nil
	}})

	got, err := g.PRMergeContext(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	want := domain.MergeContext{PullRequest: "PR_1", Clean: true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PRMergeContext() = %+v, want %+v", got, want)
	}
}

// TestMergePRPassesTheConvertedMethod wires MergePR through the gateway end
// to end.
func TestMergePRPassesTheConvertedMethod(t *testing.T) {
	t.Parallel()

	var got gql.MergeMethod
	g := New(fakeMerger{mergePR: func(_ string, method gql.MergeMethod) error {
		got = method
		return nil
	}})

	if err := g.MergePR("PR_1", domain.MergeRebase); err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if got != gql.MergeMethodRebase {
		t.Errorf("method passed to the backend = %v, want MergeMethodRebase", got)
	}
}

// TestEnableAutoMergePassesTheConvertedMethod wires EnableAutoMerge through
// the gateway end to end.
func TestEnableAutoMergePassesTheConvertedMethod(t *testing.T) {
	t.Parallel()

	var got gql.MergeMethod
	g := New(fakeMerger{enableAutoMerge: func(_ string, method gql.MergeMethod) error {
		got = method
		return nil
	}})

	if err := g.EnableAutoMerge("PR_1", domain.MergeCommit); err != nil {
		t.Fatalf("EnableAutoMerge: %v", err)
	}
	if got != gql.MergeMethodMerge {
		t.Errorf("method passed to the backend = %v, want MergeMethodMerge", got)
	}
}

func TestTheAdminPermissionSurvivesTheTranslation(t *testing.T) {
	t.Parallel()

	got := toMergeContext(gql.MergeContext{
		PullRequestID:    "PR_1",
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "BLOCKED",
		ViewerPermission: "ADMIN",
	})
	if !got.ViewerIsAdmin {
		t.Error("ViewerIsAdmin = false, want true: the popup has no other way to know")
	}
	if !got.CanMergeAsAdmin() {
		t.Error("CanMergeAsAdmin() = false, want true for a blocked pull request")
	}
}

func TestANonAdminPermissionDoesNotSetViewerIsAdmin(t *testing.T) {
	t.Parallel()

	got := toMergeContext(gql.MergeContext{
		PullRequestID:    "PR_1",
		Mergeable:        "MERGEABLE",
		MergeStateStatus: "BLOCKED",
		ViewerPermission: "WRITE",
	})
	if got.ViewerIsAdmin {
		t.Error("ViewerIsAdmin = true, want false for a WRITE permission")
	}
}
