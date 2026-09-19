package domain

import "time"

// LogLine is one line of a job's log. Time is zero on a continuation line:
// a step's output can wrap onto lines the runner did not stamp.
type LogLine struct {
	Step string
	Time time.Time
	Text string
}
