// Package domain holds the types the application is written in terms of.
// It has no behaviour beyond the rules those types carry, and it depends on
// nothing: internal/app/adapter/gateway/gh translates a service's own
// spelling into these values before anything reaches this package.
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
//
// Everything past Name and State is filled only by the per-pull-request
// query the checks view runs: the Work board's search selects the roll-up to
// count it, not to act on it.
type CheckRun struct {
	Name  string
	State CheckState
	Kind  CheckKind
	// Workflow and RunNumber name the Actions workflow this check belongs to.
	// A StatusContext belongs to none.
	Workflow  string
	RunNumber int
	// Job addresses the job's log, WorkflowRun the run to rerun. Both are
	// empty for a StatusContext, which has neither.
	Job         JobHandle
	WorkflowRun RunHandle
	// URL is where the check reports itself: detailsUrl for a check run,
	// targetUrl for a StatusContext.
	URL         string
	StartedAt   time.Time
	CompletedAt time.Time
}

// Duration is how long the check took, and zero while it is still running.
func (c CheckRun) Duration() time.Duration {
	if c.StartedAt.IsZero() || c.CompletedAt.IsZero() {
		return 0
	}
	return c.CompletedAt.Sub(c.StartedAt)
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

// RepoCount is how much is open in one repository: the badge the Repos
// sidebar puts beside its name. Unavailable says GitHub did not answer for
// this one -- it was renamed, deleted, or is no longer visible -- which is
// not the same as a repository with nothing open in it.
type RepoCount struct {
	Repo        string
	PRs, Issues int
	Unavailable bool
}

// RepoCandidate is one row of the add dialog's suggestions.
type RepoCandidate struct {
	Name    string
	Stars   int
	Private bool
}

// SavedQuery is one of the Search tab's saved queries: what the user called
// it, and the search it stands for. The query is the service's own search
// syntax, which is why it is a string and not a parsed structure.
type SavedQuery struct {
	Name  string
	Query string
}
