package gh

// FileStatus is what happened to one file in a diff.
type FileStatus int

const (
	FileModified FileStatus = iota
	FileAdded
	FileDeleted
	FileRenamed
	FileCopied
	FileChanged
	FileUnchanged
)

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
type DiffSide int

const (
	SideRight DiffSide = iota // the new file; the default for a comment
	SideLeft                  // the old file
)

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

// Hunk is one @@ block. Header is the whole @@ line as git wrote it,
// including the function context git appends after the second @@.
type Hunk struct {
	Header string
	Lines  []DiffLine
}

// FileDiff is one file's worth of a diff. OldPath is set only for a rename.
// A binary file has no hunks: git reports that it differs and nothing more.
//
// PatchOmitted is different from Binary: it means GitHub had a text diff but
// declined to send it (the files API fallback hits this for a file too
// large, among other reasons), where Binary means no text diff exists at
// all.
type FileDiff struct {
	Path         string
	OldPath      string
	Status       FileStatus
	Additions    int
	Deletions    int
	Binary       bool
	PatchOmitted bool
	Hunks        []Hunk
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
