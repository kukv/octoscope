package gql

import (
	"context"
	"slices"
	"testing"
)

func TestPRMergeContextReadsWhatTheRepositoryAllows(t *testing.T) {
	t.Parallel()

	c := fileClient(t, "testdata/merge_context.json")
	got, err := c.PRMergeContext(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	if got.PullRequestID == "" {
		t.Error("PullRequestID is empty, want the node id the mutations take")
	}
	// Measured on 2026-09-08: kukv/octoscope has all three merge methods
	// on, deleteBranchOnMerge on, and autoMergeAllowed off. The recording
	// is what says the three flags come back as read.
	if !got.SquashMergeAllowed || !got.MergeCommitAllowed || !got.RebaseMergeAllowed {
		t.Errorf("allowed methods = %+v, want all three true", got)
	}
	if got.AutoMergeAllowed {
		t.Error("AutoMergeAllowed = true, want false (measured on kukv/octoscope, spec §2)")
	}
}

func TestPRMergeContextReadsEveryField(t *testing.T) {
	t.Parallel()

	body := `{"data":{"repository":{"squashMergeAllowed":true,"mergeCommitAllowed":false,` +
		`"rebaseMergeAllowed":false,"deleteBranchOnMerge":true,"autoMergeAllowed":true,` +
		`"viewerPermission":"ADMIN",` +
		`"pullRequest":{"id":"PR_1","isDraft":false,"mergeable":"MERGEABLE",` +
		`"mergeStateStatus":"CLEAN","reviewDecision":"APPROVED",` +
		`"viewerCanEnableAutoMerge":true,"autoMergeRequest":null}}}}`
	f := &fakeSeq{outs: []string{body}}
	c := &Client{Do: f.do}

	got, err := c.PRMergeContext(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRMergeContext: %v", err)
	}
	want := MergeContext{
		PullRequestID:            "PR_1",
		IsDraft:                  false,
		Mergeable:                "MERGEABLE",
		MergeStateStatus:         "CLEAN",
		ReviewDecision:           "APPROVED",
		SquashMergeAllowed:       true,
		MergeCommitAllowed:       false,
		RebaseMergeAllowed:       false,
		DeleteBranchOnMerge:      true,
		AutoMergeAllowed:         true,
		ViewerCanEnableAutoMerge: true,
		ViewerPermission:         "ADMIN",
		AutoMergeEnabled:         false,
	}
	if got != want {
		t.Errorf("PRMergeContext() = %+v, want %+v", got, want)
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
	c := &Client{Do: f.do}

	got, err := c.PRMergeContext(context.Background(), "kukv/octoscope", 61)
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
		method MergeMethod
		want   Var
	}{
		{MergeMethodSquash, S("mergeMethod", "SQUASH")},
		{MergeMethodMerge, S("mergeMethod", "MERGE")},
		{MergeMethodRebase, S("mergeMethod", "REBASE")},
	}
	for _, tt := range tests {
		t.Run(tt.want.Str, func(t *testing.T) {
			t.Parallel()

			f := &fakeSeq{outs: []string{`{"data":{"mergePullRequest":{"pullRequest":{"merged":true}}}}`}}
			c := &Client{Do: f.do}
			if err := c.MergePR(t.Context(), "PR_1", tt.method); err != nil {
				t.Fatalf("MergePR: %v", err)
			}
			if !slices.Contains(f.calls[0], tt.want) {
				t.Errorf("call = %v, want it to carry %v", f.calls[0], tt.want)
			}
			if !slices.Contains(f.calls[0], S("pullRequestId", "PR_1")) {
				t.Errorf("call = %v, want it to carry pullRequestId=PR_1", f.calls[0])
			}
		})
	}
}

func TestAutoMergeIsTurnedOnWithAMethodAndOffWithout(t *testing.T) {
	t.Parallel()

	on := &fakeSeq{outs: []string{`{"data":{"enablePullRequestAutoMerge":{"clientMutationId":null}}}`}}
	c := &Client{Do: on.do}
	if err := c.EnableAutoMerge(t.Context(), "PR_1", MergeMethodRebase); err != nil {
		t.Fatalf("EnableAutoMerge: %v", err)
	}
	if !slices.Contains(on.calls[0], S("mergeMethod", "REBASE")) {
		t.Errorf("call = %v, want it to carry mergeMethod=REBASE", on.calls[0])
	}

	off := &fakeSeq{outs: []string{`{"data":{"disablePullRequestAutoMerge":{"clientMutationId":null}}}`}}
	c = &Client{Do: off.do}
	if err := c.DisableAutoMerge(t.Context(), "PR_1"); err != nil {
		t.Fatalf("DisableAutoMerge: %v", err)
	}
	// Turning it off takes the pull request and nothing else: the method
	// belongs to the request being cancelled, not to the cancellation.
	if slices.ContainsFunc(off.calls[0], func(v Var) bool { return v.Name == "mergeMethod" }) {
		t.Errorf("call = %v, want no mergeMethod", off.calls[0])
	}
}
