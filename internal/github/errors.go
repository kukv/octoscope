package github

import "errors"

// ErrNotInstalled is returned when the gh binary is not on PATH.
var ErrNotInstalled = errors.New("gh CLI not found; install it and run: gh auth login")

// ErrTransient wraps a failure GitHub's front end produced rather than
// answered -- 502, 503, 504. The request was well-formed, so asking again
// is the right response.
var ErrTransient = errors.New("GitHub did not answer")

// ErrUnauthenticated is returned when the client has no usable credentials.
var ErrUnauthenticated = errors.New("not authenticated; run: gh auth login")

// classified is what GitHub (or gh) said, kept apart from the sentinel that
// names what kind of failure it is. errors.Is finds the sentinel through
// Unwrap, while Error is that text and nothing else: wrapping with
// fmt.Errorf would put the sentinel's own English sentence in front of it,
// and the UI shows this text to a user who may have asked for another
// language (.claude/rules/errors.md leaves only what GitHub said
// untranslated).
type classified struct {
	kind error
	msg  string
}

func (e *classified) Error() string { return e.msg }
func (e *classified) Unwrap() error { return e.kind }

// Classify pairs what gh or GitHub said with the sentinel that says what it
// was.
func Classify(kind error, msg string) error {
	return &classified{kind: kind, msg: msg}
}
