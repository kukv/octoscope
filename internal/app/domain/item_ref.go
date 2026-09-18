package domain

// ItemKind separates pull requests from issues in a mixed list.
type ItemKind int

const (
	ItemPR ItemKind = iota
	ItemIssue
)

// ItemRef names one pull request or issue. Repo is "owner/name": both the
// Work board and the Repos tab can open an item from another repository, so
// the reference carries its own.
type ItemRef struct {
	Kind   ItemKind
	Repo   string
	Number int
}
