package domain

// DiffLine is one line of a hunk. OldLine and NewLine are that line's number
// in each version, and 0 where the version has no such line: a removed line
// has no NewLine, an added one no OldLine. Both are needed because posting a
// comment names a line number *and* the side it is on.
type DiffLine struct {
	Kind    DiffLineKind
	OldLine int
	NewLine int
	Text    string
}

// Line returns the number to quote when commenting on a line, and the side
// it is on. A context line is quoted on the right: that is the version the
// comment is about.
func (l DiffLine) Line() (int, DiffSide) {
	if l.Kind == LineRemoved {
		return l.OldLine, SideLeft
	}
	return l.NewLine, SideRight
}

// DiffLineKind separates the three kinds of line a unified diff holds.
type DiffLineKind int

const (
	LineContext DiffLineKind = iota
	LineAdded
	LineRemoved
)

// DiffSide names which version of a file a line or a comment belongs to.
// GitHub spells these LEFT and RIGHT; nothing outside the access layer sees
// those words.
//
// DiffSide is which version of a file a line or a comment belongs to.
// It lives beside DiffLine because Line below is the only place that decides
// a side; ReviewThread and PendingComment carry that answer, they do not make
// it.
type DiffSide int

const (
	SideRight DiffSide = iota // the new file; the default for a comment
	SideLeft                  // the old file
)
