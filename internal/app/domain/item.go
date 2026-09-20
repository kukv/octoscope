package domain

import "time"

type Author struct {
	Login string
}

type Label struct {
	Name  string
	Color string
}

type Comment struct {
	Author    Author
	Body      string
	CreatedAt time.Time
}

// ItemState is whether a pull request or an issue is still open, translated
// out of the strings GitHub uses so that no view switches on API spelling.
type ItemState int

const (
	StateOpen ItemState = iota
	StateClosed
	StateMerged
)

// PR and Issue carry no JSON tags: what a backend receives is that backend's
// business, and the gateway translates GitHub's own spelling into the values
// above before handing anything over.
type PR struct {
	Number    int
	Title     string
	Author    Author
	State     ItemState
	IsDraft   bool
	UpdatedAt time.Time
	Review    ReviewState
	URL       string
	// Body is the markdown the author wrote, which the detail view renders.
	// BodyText is the same text with the markdown stripped, which the drawer
	// previews. They come from different queries: a listed item carries only
	// BodyText, and one fetched on its own only Body.
	Body      string
	BodyText  string
	Comments  []Comment
	Labels    []Label
	Assignees []Author
	Checks    Checks
	// Head and Base are the branches the pull request moves between, and
	// Additions and Deletions the size of the change.
	Head      string
	Base      string
	Additions int
	Deletions int
}

type Issue struct {
	Number    int
	Title     string
	Author    Author
	State     ItemState
	UpdatedAt time.Time
	URL       string
	// Body and BodyText come from different queries, as a PR's do.
	Body      string
	BodyText  string
	Comments  []Comment
	Labels    []Label
	Assignees []Author
}

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
