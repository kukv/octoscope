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

// ItemRef is an Item's identity: which repository it is in, and which number
// it has there. Repo is "owner/name", and the reference carries its own
// because both the Work board and the Repos tab can open an item from
// another repository.
//
// Kind is part of the identity rather than of the Item because a reference
// is held before the Item is fetched -- a card on the Work board sends one
// to open a view, and what it points at has to be known by then.
type ItemRef struct {
	Kind   ItemKind
	Repo   string
	Number int
}

// Item is one pull request or issue: the thing this application is about.
// A view that does not care which of the two it has holds an Item; one that
// does asks Ref.Kind, or reads Change.
type Item struct {
	Ref    ItemRef
	Title  string
	Author Author
	State  ItemState
	URL    string
	// Body is the markdown the author wrote, which the detail view renders.
	// BodyText is the same text with the markdown stripped, which the drawer
	// previews. They come from different queries: a listed item carries only
	// BodyText, and one fetched on its own only Body.
	Body      string
	BodyText  string
	Comments  []Comment
	Labels    []Label
	Assignees []Author
	UpdatedAt time.Time

	// Change is the part only a pull request has: it proposes a change to the
	// repository. It is non-nil if and only if Ref.Kind is ItemPR, which the
	// gateway guarantees when it builds an Item.
	Change *Change
}

// Change is what a pull request adds to an item: the branches it moves
// between, how big it is, and what review and CI have said about it.
type Change struct {
	IsDraft   bool
	Review    ReviewState
	Head      string
	Base      string
	Additions int
	Deletions int
	Checks    Checks
}
