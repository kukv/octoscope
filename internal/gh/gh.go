// Package gh holds the domain types the GitHub access layer returns.
// It has no behaviour: the backends live in subpackages (cli, and api in a
// later phase) and both speak these types.
package gh

import (
	"errors"
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

type PR struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	Author         Author    `json:"author"`
	State          string    `json:"state"`
	IsDraft        bool      `json:"isDraft"`
	UpdatedAt      time.Time `json:"updatedAt"`
	ReviewDecision string    `json:"reviewDecision"`
	URL            string    `json:"url"`
	Body           string    `json:"body"`
	Comments       []Comment `json:"comments"`
	Labels         []Label   `json:"labels"`
	Assignees      []Author  `json:"assignees"`
}

type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Author    Author    `json:"author"`
	State     string    `json:"state"`
	UpdatedAt time.Time `json:"updatedAt"`
	URL       string    `json:"url"`
	Body      string    `json:"body"`
	Comments  []Comment `json:"comments"`
	Labels    []Label   `json:"labels"`
	Assignees []Author  `json:"assignees"`
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

// Checks counts the check runs behind CheckState so a progress bar can be
// drawn without a second request.
type Checks struct {
	Total   int
	Passed  int
	Failed  int
	Running int
	State   CheckState
}

// WorkItem is one card on the Work board.
type WorkItem struct {
	Ref       ItemRef
	Title     string
	Author    string
	IsDraft   bool
	Labels    []Label
	Review    ReviewState
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
)

// WorkSections returns the columns in display order, left to right.
func WorkSections() []WorkSection {
	return []WorkSection{
		SectionReviewRequested,
		SectionYourPRs,
		SectionAssigned,
		SectionMentioned,
	}
}

// Work holds the items of each column, indexed by WorkSection.
type Work [4][]WorkItem
