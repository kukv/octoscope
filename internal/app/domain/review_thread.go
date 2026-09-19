package domain

import "slices"

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

// Pending reports whether this thread contains any unsubmitted comment.
// A pending reply inside an otherwise public thread is still pending.
func (t ReviewThread) Pending() bool {
	return slices.ContainsFunc(t.Comments, func(c ThreadComment) bool { return c.Pending })
}

// Collapsed reports whether the thread is drawn as a count rather than in
// full. Settled conversations must not push the code they were about off the
// screen.
func (t ReviewThread) Collapsed() bool { return t.Resolved || t.Outdated }
