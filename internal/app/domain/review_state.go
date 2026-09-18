package domain

// ReviewState is the review outcome of a pull request, translated out of the
// GraphQL reviewDecision enum so the UI never switches on API spelling.
type ReviewState int

const (
	ReviewNone ReviewState = iota
	ReviewRequired
	ReviewApproved
	ReviewChangesRequested
)
