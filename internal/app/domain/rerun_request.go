package domain

// RerunRequest is what a rerun request names: the run to start again, and
// how much of it. The two travel together -- a scope with no run names
// nothing -- so they are one type rather than two arguments.
type RerunRequest struct {
	Run   RunHandle
	Scope RerunScope
}

// RerunScope is how much of a workflow run to start again.
type RerunScope int

const (
	RerunFailed RerunScope = iota
	RerunAll
)
