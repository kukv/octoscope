package domain

// ReviewEvent is what submitting a review says about it.
type ReviewEvent int

const (
	EventComment ReviewEvent = iota
	EventApprove
	EventRequestChanges
)
