package domain

// RepoCount is how much is open in one repository: the badge the Repos
// sidebar puts beside its name. Unavailable says GitHub did not answer for
// this one -- it was renamed, deleted, or is no longer visible -- which is
// not the same as a repository with nothing open in it.
type RepoCount struct {
	Repo        string
	PRs, Issues int
	Unavailable bool
}
