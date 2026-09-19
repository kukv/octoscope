package domain

// RepoCandidate is one row of the add dialog's suggestions.
type RepoCandidate struct {
	Name    string
	Stars   int
	Private bool
}
