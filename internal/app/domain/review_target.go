package domain

// ReviewTarget names the pull request a review acts on, and the unsubmitted
// review already open on it if there is one. GitHub allows one unsubmitted
// review per user per pull request, so Pending is a single handle, not a
// list.
type ReviewTarget struct {
	PullRequest PullRequestHandle
	Pending     ReviewHandle
}
