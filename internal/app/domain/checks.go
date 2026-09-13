package domain

import (
	"strings"
	"time"
)

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

// NewLogLine reads one line of a job's log. message is what the runner wrote:
// the stamp it prefixes most lines with, and on the very first line of a log a
// byte order mark before that. Neither is part of what the step printed.
func NewLogLine(step, message string) LogLine {
	message = strings.TrimPrefix(message, "\ufeff")
	line := LogLine{Step: step, Text: message}
	if stamp, rest, ok := strings.Cut(message, " "); ok {
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			line.Time, line.Text = at, rest
		}
	}
	return line
}

// RerunScope is how much of a workflow run to start again.
type RerunScope int

const (
	RerunFailed RerunScope = iota
	RerunAll
)
