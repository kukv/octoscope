package domain

import "time"

// WorkItem is one card on the Work board.
type WorkItem struct {
	Ref     ItemRef
	Title   string
	Body    string
	Author  string
	IsDraft bool
	// State is open, closed or merged. The Work board's own searches are
	// all is:open; a search the user wrote is not.
	State  ItemState
	Labels []Label
	Review ReviewState
	// Head and Base are the branches a pull request moves between, and
	// Additions and Deletions the size of the change. All four are empty for
	// an issue.
	Head      string
	Base      string
	Additions int
	Deletions int
	Checks    Checks
	UpdatedAt time.Time
	URL       string
}
