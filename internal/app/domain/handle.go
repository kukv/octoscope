package domain

// A handle addresses something on the service the application is talking to.
// The application never reads one: it passes back what it was given. The
// service's own shape for an id -- a node id, a database id -- stops at the
// gateway.
//
// A handle is never constructed here. The gateway mints one while it builds
// the value a fetch answers with, so the only way to hold a handle is to
// have fetched the thing it addresses.
type (
	// PullRequestHandle addresses one pull request. So does ItemRef, and
	// the two are not two spellings of one thing: reading and the writes on
	// item.go take a ref, reviewing and merging take a handle, and neither
	// can be derived from the other. A handle carries no repository and no
	// number to read back, and a ref cannot be turned into one without
	// asking the service.
	//
	// PRReviewContext and PRMergeContext each hand out their own, so the
	// handle held while reviewing and the handle held while merging are
	// separate values that happen to address the same pull request. Nothing
	// relates them, which is why the diff view cannot pass what it has to
	// the merge popup.
	//
	// The two are not folded into one because folding them means resolving
	// a ref to a handle somewhere, and that resolution is a request. A port
	// whose signature hides a request is what architecture.md (ii) rules
	// out: the call count has to stay readable from the port.
	PullRequestHandle string
	// ReviewHandle addresses one review, submitted or not.
	ReviewHandle string
	// JobHandle addresses one job's log. Empty for a check that is not a
	// job, which has no log to read.
	JobHandle string
	// RunHandle addresses one workflow run. Empty for a check that belongs
	// to no run, which is nothing to rerun.
	RunHandle string
)
