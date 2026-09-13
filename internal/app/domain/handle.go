package domain

// A handle addresses something on the service the application is talking to.
// The application never reads one: it passes back what it was given. The
// service's own shape for an id -- a node id, a database id -- stops at the
// gateway.
type (
	// PullRequestHandle addresses one pull request.
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
