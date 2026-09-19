package domain

// MergeMethod is how the pull request's commits land on the base branch.
type MergeMethod int

const (
	MergeSquash MergeMethod = iota
	MergeCommit
	MergeRebase
)

// Mergeable is GitHub's answer to "can this be merged". Unknown is an
// ordinary state, not a failure: GitHub returns it while it is still
// working the answer out.
type Mergeable int

const (
	MergeableUnknown Mergeable = iota
	MergeableYes
	MergeableConflicting
)

// MergeState is the finer answer, translated out of the GraphQL
// mergeStateStatus enum so the UI never switches on API spelling.
type MergeState int

const (
	MergeStateUnknown MergeState = iota
	MergeStateClean
	MergeStateBlocked
	MergeStateBehind
	MergeStateDirty
	MergeStateUnstable
	MergeStateHasHooks
)

// MergeBlock is why merging is refused right now. BlockNone means it is not.
type MergeBlock int

const (
	BlockNone MergeBlock = iota
	BlockDraft
	BlockConflicting
	BlockComputing
	BlockProtected
	BlockBehind
	BlockDirty
)

// MergeContext is everything the merge popup draws and acts on: what the
// repository allows, and what state this pull request is in.
type MergeContext struct {
	PullRequest PullRequestHandle
	IsDraft     bool
	Mergeable   Mergeable
	State       MergeState
	Review      ReviewState

	// Methods holds only the methods the repository allows, in the order
	// the popup lists them: squash, merge commit, rebase.
	Methods             []MergeMethod
	DeleteBranchOnMerge bool

	AutoMergeAllowed         bool
	ViewerCanEnableAutoMerge bool
	AutoMergeEnabled         bool

	// ViewerIsAdmin is what keeps the popup from offering a key that fails.
	// The mutation that merges takes no admin input -- gh pr merge --admin
	// sends the same one -- so nothing in the answer to the merge itself
	// says whether this viewer may push past a rule. GitHub's own
	// viewerCanMergeAsAdmin reads classic branch protection only and answers
	// false under a ruleset, so the gateway fills this from the repository
	// permission instead.
	ViewerIsAdmin bool
}

// Block says why merging is refused. Draft comes first: GitHub reports a
// draft as BLOCKED, and "it is a draft" is the more useful of the two.
func (c MergeContext) Block() MergeBlock {
	switch {
	case c.IsDraft:
		return BlockDraft
	case c.Mergeable == MergeableConflicting:
		return BlockConflicting
	case c.Mergeable == MergeableUnknown:
		return BlockComputing
	case c.State == MergeStateBlocked:
		return BlockProtected
	case c.State == MergeStateBehind:
		return BlockBehind
	case c.State == MergeStateDirty:
		return BlockDirty
	}
	return BlockNone
}

// CanAutoMerge reports whether auto-merge can be turned on. GitHub refuses
// it on a pull request that is already clean: there is nothing left to wait
// for, so it wants an ordinary merge instead.
func (c MergeContext) CanAutoMerge() bool {
	return c.AutoMergeAllowed && c.ViewerCanEnableAutoMerge && c.State != MergeStateClean
}

// CanMergeAsAdmin reports whether the viewer can push the merge through what
// is holding it. Only two blocks give way, the same two gh's --admin silences.
// A draft or a conflict is not a rule to bypass: it is work that is not
// finished, and no permission finishes it.
func (c MergeContext) CanMergeAsAdmin() bool {
	if !c.ViewerIsAdmin {
		return false
	}
	switch c.Block() {
	case BlockProtected, BlockBehind:
		return true
	}
	return false
}
