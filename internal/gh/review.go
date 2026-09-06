package gh

import "time"

// ThreadComment is one comment inside a review thread. Pending marks a
// comment the viewer has written but not submitted: GitHub returns it in the
// same place as everyone else's, and only the state of the review it belongs
// to tells the two apart.
type ThreadComment struct {
	Author    Author
	Body      string
	CreatedAt time.Time
	Pending   bool
}

// ReviewThread is one conversation attached to a line of the diff.
//
// Line is the line it sits on in the version named by Side. GitHub returns no
// line for a thread whose code has since moved, in which case Outdated is set
// and Line is the line it was originally written against.
type ReviewThread struct {
	Path     string
	Line     int
	Side     DiffSide
	Resolved bool
	Outdated bool
	Comments []ThreadComment
}

// Pending reports whether this thread is one the viewer has not submitted.
// Such a thread has exactly one comment, and it is theirs.
func (t ReviewThread) Pending() bool {
	return len(t.Comments) > 0 && t.Comments[0].Pending
}

// Collapsed reports whether the thread is drawn as a count rather than in
// full. Settled conversations must not push the code they were about off the
// screen (spec 4.4.1).
func (t ReviewThread) Collapsed() bool { return t.Resolved || t.Outdated }

// PendingComment is a line comment on its way to GitHub.
type PendingComment struct {
	Path string
	Line int
	Side DiffSide
	Body string
}

// ReviewEvent is what submitting a review says about it.
type ReviewEvent int

const (
	EventComment ReviewEvent = iota
	EventApprove
	EventRequestChanges
)

// ReviewContext is everything the diff view needs before it can draw or
// change a review: the pull request's node id, what its header says, the
// unsubmitted review if there is one, and the threads already on the diff.
//
// The header belongs here rather than being handed in by whoever opened the
// view, because a card on the Work board and a row in the Repos tab know
// different amounts about the pull request, and neither knows all of it.
type ReviewContext struct {
	PullRequestID string
	Title         string
	Head          string
	Base          string
	Additions     int
	Deletions     int
	// PendingID is the unsubmitted review's node id, empty when there is
	// none. A pending review is visible only to its author, so anything that
	// comes back here belongs to the viewer.
	PendingID string
	Threads   []ReviewThread
}
