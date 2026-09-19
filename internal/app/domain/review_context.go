package domain

// ReviewContext is everything the diff view needs before it can draw or
// change a review: the pull request it belongs to, what its header says, the
// unsubmitted review if there is one, and the threads already on the diff.
//
// The header belongs here rather than being handed in by whoever opened the
// view, because a card on the Work board and a row in the Repos tab know
// different amounts about the pull request, and neither knows all of it.
type ReviewContext struct {
	PullRequest PullRequestHandle
	Title       string
	Head        string
	Base        string
	Additions   int
	Deletions   int
	// Pending addresses the unsubmitted review, empty when there is none. A
	// pending review is visible only to its author, so anything that comes
	// back here belongs to the viewer.
	Pending ReviewHandle
	Threads []ReviewThread
}

// PendingCount is how many comments across every thread have not been
// submitted yet -- what a review submission is about to send along with it.
func (c ReviewContext) PendingCount() int {
	n := 0
	for _, t := range c.Threads {
		for _, comment := range t.Comments {
			if comment.Pending {
				n++
			}
		}
	}
	return n
}
