package domain

import "testing"

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
			Block: BlockProtected,
		}), true},
		{"behind: gh's --admin bypasses this one too", admin(MergeContext{
			Block: BlockBehind,
		}), true},
		{"clean: nothing to push past", admin(MergeContext{
			Block: BlockNone, Clean: true,
		}), false},
		{"unstable: GitHub allows the merge already", admin(MergeContext{
			Block: BlockNone,
		}), false},
		{"draft: not a rule, unfinished work", admin(MergeContext{
			Block: BlockDraft,
		}), false},
		{"conflicting: no permission resolves a conflict", admin(MergeContext{
			Block: BlockConflicting,
		}), false},
		{"computing: the answer is not in yet", admin(MergeContext{
			Block: BlockComputing,
		}), false},
		{"dirty: GitHub will not merge it at all", admin(MergeContext{
			Block: BlockDirty,
		}), false},
		{"blocked, but the viewer is no admin", MergeContext{
			Block: BlockProtected,
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
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, Clean: false,
		}, true},
		{"nothing to wait for: GitHub refuses auto-merge on a clean pull request", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: true, Clean: true,
		}, false},
		{"repository has it turned off", MergeContext{
			AutoMergeAllowed: false, ViewerCanEnableAutoMerge: true, Clean: false,
		}, false},
		{"viewer may not", MergeContext{
			AutoMergeAllowed: true, ViewerCanEnableAutoMerge: false, Clean: false,
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
