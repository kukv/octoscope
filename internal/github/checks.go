package github

import (
	"strings"
	"time"
)

// LogLine is one line of a job's log, as GitHub Actions writes it. Time is
// zero on a continuation line: a step's output can wrap onto lines the
// runner did not stamp.
type LogLine struct {
	Step string
	Time time.Time
	Text string
}

// ParseLogLine reads one line of a job's log. message is what the runner
// wrote: the stamp it prefixes most lines with, and on the very first line
// of a log a byte order mark before that. Neither is part of what the step
// printed.
func ParseLogLine(step, message string) LogLine {
	message = strings.TrimPrefix(message, "\ufeff")
	line := LogLine{Step: step, Text: message}
	if stamp, rest, ok := strings.Cut(message, " "); ok {
		if at, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
			line.Time, line.Text = at, rest
		}
	}
	return line
}

type RerunScope int

const (
	RerunFailed RerunScope = iota
	RerunAll
)
