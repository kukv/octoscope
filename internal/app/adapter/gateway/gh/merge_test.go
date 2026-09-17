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
		IsDraft:                  true,
		Mergeable:                domain.MergeableConflicting,
		State:                    domain.MergeStateDirty,
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

// TestParseMergeableCoversEveryValue guards every domain.Mergeable value.
// A value this switch does not recognise reads as MergeableUnknown, which
// the popup treats as "still computing" rather than a fetch failure.
func TestParseMergeableCoversEveryValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mergeable string
		want      domain.Mergeable
	}{
		{"UNKNOWN", domain.MergeableUnknown},
		{"MERGEABLE", domain.MergeableYes},
		{"CONFLICTING", domain.MergeableConflicting},
		{"a word we do not know is not a failure", domain.MergeableUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.mergeable, func(t *testing.T) {
			t.Parallel()

			if got := parseMergeable(tt.mergeable); got != tt.want {
				t.Errorf("parseMergeable(%q) = %v, want %v", tt.mergeable, got, tt.want)
			}
		})
	}
}

// TestParseMergeStateCoversEveryValue guards every domain.MergeState value,
// including the ones a mergeStateStatus enum a stray string does not know:
// misreading BLOCKED as CLEAN is the difference between telling a user
// their pull request is ready and telling them it is blocked.
func TestParseMergeStateCoversEveryValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state string
		want  domain.MergeState
	}{
		{"UNKNOWN", domain.MergeStateUnknown},
		{"CLEAN", domain.MergeStateClean},
		{"BLOCKED", domain.MergeStateBlocked},
		{"BEHIND", domain.MergeStateBehind},
		{"DIRTY", domain.MergeStateDirty},
		{"UNSTABLE", domain.MergeStateUnstable},
		{"HAS_HOOKS", domain.MergeStateHasHooks},
		{"a word we do not know is not a failure", domain.MergeStateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			t.Parallel()

			if got := parseMergeState(tt.state); got != tt.want {
				t.Errorf("parseMergeState(%q) = %v, want %v", tt.state, got, tt.want)
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
	want := domain.MergeContext{PullRequest: "PR_1", Mergeable: domain.MergeableYes, State: domain.MergeStateClean}
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
