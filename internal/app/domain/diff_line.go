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
// It lives beside DiffLine, although ReviewThread and PendingComment carry a
// side too, because Line above is the only place that decides one; the other
// two carry that answer rather than making it.
type DiffSide int

const (
	SideRight DiffSide = iota // the new file; the default for a comment
	SideLeft                  // the old file
)
