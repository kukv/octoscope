// Package gh holds the domain types the GitHub access layer returns.
// It has no behaviour: the backends live in subpackages (cli, and api in a
// later phase) and both speak these types.
package gh

import (
	"errors"
	"strings"
	"time"
)

// ErrGhNotFound is returned when the gh binary is not on PATH.
var ErrGhNotFound = errors.New("gh CLI not found; install it and run: gh auth login")

type Author struct {
	Login string `json:"login"`
}

type Label struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Comment struct {
	Author    Author    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
}

// ItemState is whether a pull request or an issue is still open, translated
// out of the strings GitHub uses so that no view switches on API spelling.
type ItemState int

const (
	StateOpen ItemState = iota
	StateClosed
	StateMerged
)

// ParseItemState maps GitHub's state onto the domain value. GraphQL and REST
// differ in case, so the comparison ignores it; anything unrecognised reads
// as closed, which is the reading that offers no action.
func ParseItemState(state string) ItemState {
	switch strings.ToUpper(state) {
	case "OPEN":
		return StateOpen
	case "MERGED":
		return StateMerged
	default:
		return StateClosed
	}
}

// PR and Issue carry no JSON tags: what a backend receives is that backend's
// business, and both of them translate GitHub's own spelling into the values
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

type Issue struct {
	Number    int
	Title     string
	Author    Author
	State     ItemState
	UpdatedAt time.Time
	URL       string
	Body      string
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

// ReviewState is the review outcome of a pull request, translated out of the
// GraphQL reviewDecision enum so the UI never switches on API spelling.
type ReviewState int

const (
	ReviewNone ReviewState = iota
	ReviewRequired
	ReviewApproved
	ReviewChangesRequested
)

// ParseReviewDecision maps the GraphQL reviewDecision enum onto the domain
// value. An empty string means the pull request needs no review at all; an
// unknown one is treated the same way rather than failing the whole fetch.
func ParseReviewDecision(decision string) ReviewState {
	switch decision {
	case "APPROVED":
		return ReviewApproved
	case "CHANGES_REQUESTED":
		return ReviewChangesRequested
	case "REVIEW_REQUIRED":
		return ReviewRequired
	default:
		return ReviewNone
	}
}

// CheckState is the rolled-up outcome of a pull request's checks.
type CheckState int

const (
	CheckNone CheckState = iota
	CheckPending
	CheckRunning
	CheckSuccess
	CheckFailure
)

// CheckRun is one check behind the roll-up, named as GitHub names it.
type CheckRun struct {
	Name  string
	State CheckState
}

// Checks counts the check runs behind CheckState so a progress bar can be
// drawn without a second request, and keeps them so the drawer can list them
// without one either.
type Checks struct {
	Total   int
	Passed  int
	Failed  int
	Running int
	State   CheckState
	Runs    []CheckRun
}

// WorkItem is one card on the Work board.
type WorkItem struct {
	Ref     ItemRef
	Title   string
	Body    string
	Author  string
	IsDraft bool
	Labels  []Label
	Review  ReviewState
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

// WorkSection is one column of the Work board.
type WorkSection int

const (
	SectionReviewRequested WorkSection = iota
	SectionYourPRs
	SectionAssigned
	SectionMentioned

	// WorkSectionCount must be the last constant in this block: Work's
	// length as an array indexed by WorkSection comes from it.
	WorkSectionCount = iota
)

// WorkSections returns the columns in display order, left to right.
func WorkSections() []WorkSection {
	sections := make([]WorkSection, WorkSectionCount)
	for i := range sections {
		sections[i] = WorkSection(i)
	}
	return sections
}

// Work holds the items of each column, indexed by WorkSection.
type Work [WorkSectionCount][]WorkItem
