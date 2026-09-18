package domain

import "time"

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
	Body      string
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
