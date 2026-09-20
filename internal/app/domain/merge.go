package domain

// MergeMethod is how the pull request's commits land on the base branch.
type MergeMethod int

const (
	MergeSquash MergeMethod = iota
	MergeCommit
	MergeRebase
)

// MergeBlock is why merging is refused right now. BlockNone means it is not.
// The gateway decides which one applies: reading a service's own answer into
// this is translation, not a rule of this application.
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

	// Block is why merging is refused right now, as the gateway read it out
	// of what the service reported. BlockNone means nothing refuses it.
	Block MergeBlock

	// Clean says there is nothing left to wait for. That is not the same as
	// "not blocked": checks still running refuse nothing, and auto-merge is
	// exactly the thing to offer then.
	Clean bool

	Review ReviewState

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

// CanAutoMerge reports whether auto-merge can be turned on. GitHub refuses
// it on a pull request that is already clean: there is nothing left to wait
// for, so it wants an ordinary merge instead.
func (c MergeContext) CanAutoMerge() bool {
	return c.AutoMergeAllowed && c.ViewerCanEnableAutoMerge && !c.Clean
}

// CanMergeAsAdmin reports whether the viewer can push the merge through what
// is holding it. Only two blocks give way, the same two gh's --admin silences.
// A draft or a conflict is not a rule to bypass: it is work that is not
// finished, and no permission finishes it.
func (c MergeContext) CanMergeAsAdmin() bool {
	if !c.ViewerIsAdmin {
		return false
	}
	switch c.Block {
	case BlockProtected, BlockBehind:
		return true
	}
	return false
}
