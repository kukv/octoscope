package gh

import "testing"

func TestBlockNamesWhyMergingIsRefused(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  MergeContext
		want MergeBlock
	}{
		{"clean", MergeContext{Mergeable: MergeableYes, State: MergeStateClean}, BlockNone},
		{
			"draft outranks the state GitHub reports for it",
			MergeContext{IsDraft: true, Mergeable: MergeableYes, State: MergeStateBlocked},
			BlockDraft,
		},
		{"conflicting", MergeContext{Mergeable: MergeableConflicting, State: MergeStateDirty}, BlockConflicting},
		{"still computing", MergeContext{Mergeable: MergeableUnknown, State: MergeStateUnknown}, BlockComputing},
		{"protected", MergeContext{Mergeable: MergeableYes, State: MergeStateBlocked}, BlockProtected},
		{"behind", MergeContext{Mergeable: MergeableYes, State: MergeStateBehind}, BlockBehind},
		{"dirty", MergeContext{Mergeable: MergeableYes, State: MergeStateDirty}, BlockDirty},
		{
			"failing checks do not block: GitHub allows the merge",
			MergeContext{Mergeable: MergeableYes, State: MergeStateUnstable},
			BlockNone,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ctx.Block(); got != tt.want {
				t.Errorf("Block() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAutoMergeNeedsSomethingToWaitFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ctx  MergeContext
		want bool
	}{
		{"waiting on checks", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, State: MergeStateUnstable,
		}, true},
		{"nothing to wait for: GitHub refuses auto-merge on a clean pull request", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, State: MergeStateClean,
		}, false},
		{"repository has it turned off", MergeContext{
			AutoMergeAllowed: false, ViewerCanEnableAutoMerge: true, State: MergeStateUnstable,
		}, false},
		{"viewer may not", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: false, State: MergeStateUnstable,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ctx.CanAutoMerge(); got != tt.want {
				t.Errorf("CanAutoMerge() = %v, want %v", got, tt.want)
			}
		})
	}
}
