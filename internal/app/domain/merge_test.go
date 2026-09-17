package domain

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

func TestOnlyTwoBlocksGiveWayToAnAdmin(t *testing.T) {
	t.Parallel()

	// admin makes a context in the given state with the permission granted:
	// every row below turns on the permission, so what the table measures is
	// the block, not the flag. The one row that turns it off is last.
	admin := func(c MergeContext) MergeContext {
		c.ViewerIsAdmin = true
		return c
	}

	tests := []struct {
		name string
		ctx  MergeContext
		want bool
	}{
		{"protected: this is what the key is for", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateBlocked,
		}), true},
		{"behind: gh's --admin bypasses this one too", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateBehind,
		}), true},
		{"clean: nothing to push past", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateClean,
		}), false},
		{"unstable: GitHub allows the merge already", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateUnstable,
		}), false},
		{"draft: not a rule, unfinished work", admin(MergeContext{
			IsDraft: true, Mergeable: MergeableYes, State: MergeStateBlocked,
		}), false},
		{"conflicting: no permission resolves a conflict", admin(MergeContext{
			Mergeable: MergeableConflicting, State: MergeStateDirty,
		}), false},
		{"computing: the answer is not in yet", admin(MergeContext{
			Mergeable: MergeableUnknown, State: MergeStateUnknown,
		}), false},
		{"dirty: GitHub will not merge it at all", admin(MergeContext{
			Mergeable: MergeableYes, State: MergeStateDirty,
		}), false},
		{"blocked, but the viewer is no admin", MergeContext{
			Mergeable: MergeableYes, State: MergeStateBlocked,
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.ctx.CanMergeAsAdmin(); got != tt.want {
				t.Errorf("CanMergeAsAdmin() = %v, want %v", got, tt.want)
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
