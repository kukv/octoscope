package domain

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
