package domain

import "time"

// CheckKind separates the two shapes GitHub reports a check in. Only a check
// run has a log to read and a workflow to rerun; a StatusContext is an
// external service reporting a state and a link.
type CheckKind int

const (
	CheckKindRun CheckKind = iota
	CheckKindStatus
)

// LogLine is one line of a job's log. Time is zero on a continuation line:
// a step's output can wrap onto lines the runner did not stamp.
type LogLine struct {
	Step string
	Time time.Time
	Text string
}

// RerunScope is how much of a workflow run to start again.
type RerunScope int

const (
	RerunFailed RerunScope = iota
	RerunAll
)

// CheckState is the rolled-up outcome of a pull request's checks.
type CheckState int

const (
	CheckNone CheckState = iota
	CheckPending
	CheckRunning
	CheckSuccess
	CheckFailure
)

// CheckRun is one check behind the roll-up, named as GitHub names it.
//
// Everything past Name and State is filled only by the per-pull-request
// query the checks view runs: the Work board's search selects the roll-up to
// count it, not to act on it.
type CheckRun struct {
	Name  string
	State CheckState
	Kind  CheckKind
	// Workflow and RunNumber name the Actions workflow this check belongs to.
	// A StatusContext belongs to none.
	Workflow  string
	RunNumber int
	// Job addresses the job's log, WorkflowRun the run to rerun. Both are
	// empty for a StatusContext, which has neither.
	Job         JobHandle
	WorkflowRun RunHandle
	// URL is where the check reports itself: detailsUrl for a check run,
	// targetUrl for a StatusContext.
	URL         string
	StartedAt   time.Time
	CompletedAt time.Time
}

// Duration is how long the check took, and zero while it is still running.
func (c CheckRun) Duration() time.Duration {
	if c.StartedAt.IsZero() || c.CompletedAt.IsZero() {
		return 0
	}
	return c.CompletedAt.Sub(c.StartedAt)
}

// Checks counts the check runs behind CheckState so a progress bar can be
// drawn without a second request, and keeps them so the drawer can list them
// without one either.
type Checks struct {
	Total   int
	Passed  int
	Failed  int
	Running int
	State   CheckState
	Runs    []CheckRun
}
