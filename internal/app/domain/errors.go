package domain

import "errors"

// ErrBackendUnavailable is returned when the backend that talks to GitHub
// cannot be reached at all -- the gh binary missing is one backend's
// problem, not a thing the application itself knows about. The text names
// only the kind: what the user should do about it is the UI's to say, in
// their language (i18n error.gh_not_found).
var ErrBackendUnavailable = errors.New("backend unavailable")

// ErrTransient wraps a failure GitHub's front end produced rather than
// answered -- 502, 503, 504. The request was well-formed, so asking again
// is the right response.
var ErrTransient = errors.New("GitHub did not answer")

// ErrUnauthenticated is returned when the backend has no usable credentials
// -- gh not signed in, or no token for the API client. Same rule as above:
// the remedy is i18n error.unauthenticated, not this string.
var ErrUnauthenticated = errors.New("not authenticated")

// classified is what gh said, kept apart from the sentinel that names what
// kind of failure it is. errors.Is finds the sentinel through Unwrap, while
// Error is gh's own text and nothing else: wrapping with fmt.Errorf would put
// the sentinel's English sentence in front of it, and the UI shows this text
// to a user who may have asked for another language (.claude/rules/errors.md
// leaves only what GitHub said untranslated).
type classified struct {
	kind error
	msg  string
}

func (e *classified) Error() string { return e.msg }
func (e *classified) Unwrap() error { return e.kind }

// Classify pairs what gh said with the sentinel that says what it was.
func Classify(kind error, msg string) error {
	return &classified{kind: kind, msg: msg}
}

// IsFatal reports whether the user has to act before anything can work. Every
// other failure is worth a line above the key bar and another try.
//
// It lives here rather than in each view because the sentinels it asks about
// are this package's own: a second copy of the list would be free to fall
// behind the sentinels it names, and the Work board and the Repos list would
// then disagree about what costs the user their screen.
func IsFatal(err error) bool {
	return errors.Is(err, ErrBackendUnavailable) || errors.Is(err, ErrUnauthenticated)
}
