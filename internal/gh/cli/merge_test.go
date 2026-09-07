package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func TestPRMergeContextReadsWhatTheRepositoryAllows(t *testing.T) {
	t.Parallel()

	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: fileRun(t, "testdata/merge_context.json")}
	got, err := c.PRMergeContext(t.Context(), "", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	if got.PullRequestID == "" {
		t.Error("PullRequestID is empty, want the node id the mutations take")
	}
	// Measured on 2026-09-08: kukv/octoscope has all three merge methods
	// on, deleteBranchOnMerge on, and autoMergeAllowed off. The recording
	// is what says the three flags are read into the order the popup lists
	// them in.
	want := []gh.MergeMethod{gh.MergeSquash, gh.MergeCommit, gh.MergeRebase}
	if len(got.Methods) != len(want) {
		t.Fatalf("Methods = %v, want %v", got.Methods, want)
	}
	for i := range want {
		if got.Methods[i] != want[i] {
			t.Fatalf("Methods = %v, want %v", got.Methods, want)
		}
	}
	if got.AutoMergeAllowed {
		t.Error("AutoMergeAllowed = true, want false (measured on kukv/octoscope, spec §2)")
	}
}

func TestPRMergeContextTranslatesTheEnums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		mergeable     string
		state         string
		wantMergeable gh.Mergeable
		wantState     gh.MergeState
	}{
		{"clean", "MERGEABLE", "CLEAN", gh.MergeableYes, gh.MergeStateClean},
		{"conflicting", "CONFLICTING", "DIRTY", gh.MergeableConflicting, gh.MergeStateDirty},
		{"still computing", "UNKNOWN", "UNKNOWN", gh.MergeableUnknown, gh.MergeStateUnknown},
		{"failing checks", "MERGEABLE", "UNSTABLE", gh.MergeableYes, gh.MergeStateUnstable},
		{"protected", "MERGEABLE", "BLOCKED", gh.MergeableYes, gh.MergeStateBlocked},
		{"behind", "MERGEABLE", "BEHIND", gh.MergeableYes, gh.MergeStateBehind},
		{"hooks", "MERGEABLE", "HAS_HOOKS", gh.MergeableYes, gh.MergeStateHasHooks},
		{"a word we do not know is not a failure", "WAT", "WAT", gh.MergeableUnknown, gh.MergeStateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := `{"data":{"repository":{"squashMergeAllowed":true,"mergeCommitAllowed":false,` +
				`"rebaseMergeAllowed":false,"deleteBranchOnMerge":true,"autoMergeAllowed":true,` +
				`"pullRequest":{"id":"PR_1","isDraft":false,"mergeable":"` + tt.mergeable + `",` +
				`"mergeStateStatus":"` + tt.state + `","reviewDecision":"APPROVED",` +
				`"viewerCanEnableAutoMerge":true,"autoMergeRequest":null}}}}`
			f := &fakeSeq{outs: []string{body}}
			c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

			got, err := c.PRMergeContext(t.Context(), "", 61)
			if err != nil {
				t.Fatalf("PRMergeContext: %v", err)
			}
			if got.Mergeable != tt.wantMergeable {
				t.Errorf("Mergeable = %v, want %v", got.Mergeable, tt.wantMergeable)
			}
			if got.State != tt.wantState {
				t.Errorf("State = %v, want %v", got.State, tt.wantState)
			}
			if got.Review != gh.ReviewApproved {
				t.Errorf("Review = %v, want ReviewApproved", got.Review)
			}
			if !got.DeleteBranchOnMerge {
				t.Error("DeleteBranchOnMerge = false, want true")
			}
			if got.AutoMergeEnabled {
				t.Error("AutoMergeEnabled = true, want false (autoMergeRequest is null)")
			}
		})
	}
}

func TestPRMergeContextSeesAutoMergeAlreadyOn(t *testing.T) {
	t.Parallel()

	body := `{"data":{"repository":{"squashMergeAllowed":true,"mergeCommitAllowed":true,` +
		`"rebaseMergeAllowed":true,"deleteBranchOnMerge":false,"autoMergeAllowed":true,` +
		`"pullRequest":{"id":"PR_1","isDraft":false,"mergeable":"MERGEABLE",` +
		`"mergeStateStatus":"UNSTABLE","reviewDecision":"","viewerCanEnableAutoMerge":true,` +
		`"autoMergeRequest":{"enabledAt":"2026-09-08T01:00:00Z"}}}}}`
	f := &fakeSeq{outs: []string{body}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}

	got, err := c.PRMergeContext(t.Context(), "", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	if !got.AutoMergeEnabled {
		t.Error("AutoMergeEnabled = false, want true: the popup offers to turn it off instead")
	}
}

func TestMergePRSendsTheMethodTheUserChose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		method gh.MergeMethod
		want   string
	}{
		{gh.MergeSquash, "mergeMethod=SQUASH"},
		{gh.MergeCommit, "mergeMethod=MERGE"},
		{gh.MergeRebase, "mergeMethod=REBASE"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()

			f := &fakeSeq{outs: []string{`{"data":{"mergePullRequest":{"pullRequest":{"merged":true}}}}`}}
			c := &Client{dir: "/repo", repo: "kukv/octoscope", run: f.run}
			if err := c.MergePR("PR_1", tt.method); err != nil {
				t.Fatalf("MergePR: %v", err)
			}
			if !slices.Contains(f.calls[0], tt.want) {
				t.Errorf("call = %v, want it to carry %s", f.calls[0], tt.want)
			}
			if !slices.Contains(f.calls[0], "pullRequestId=PR_1") {
				t.Errorf("call = %v, want it to carry pullRequestId=PR_1", f.calls[0])
			}
		})
	}
}

func TestAutoMergeIsTurnedOnWithAMethodAndOffWithout(t *testing.T) {
	t.Parallel()

	on := &fakeSeq{outs: []string{`{"data":{"enablePullRequestAutoMerge":{"clientMutationId":null}}}`}}
	c := &Client{dir: "/repo", repo: "kukv/octoscope", run: on.run}
	if err := c.EnableAutoMerge("PR_1", gh.MergeRebase); err != nil {
		t.Fatalf("EnableAutoMerge: %v", err)
	}
	if !slices.Contains(on.calls[0], "mergeMethod=REBASE") {
		t.Errorf("call = %v, want it to carry mergeMethod=REBASE", on.calls[0])
	}

	off := &fakeSeq{outs: []string{`{"data":{"disablePullRequestAutoMerge":{"clientMutationId":null}}}`}}
	c = &Client{dir: "/repo", repo: "kukv/octoscope", run: off.run}
	if err := c.DisableAutoMerge("PR_1"); err != nil {
		t.Fatalf("DisableAutoMerge: %v", err)
	}
	// Turning it off takes the pull request and nothing else: the method
	// belongs to the request being cancelled, not to the cancellation.
	for _, arg := range off.calls[0] {
		if strings.HasPrefix(arg, "mergeMethod=") {
			t.Errorf("call = %v, want no mergeMethod", off.calls[0])
		}
	}
}
