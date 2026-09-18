// Package domain holds the types the application is written in terms of.
// It has no behaviour beyond the rules those types carry, and it depends on
// nothing: internal/app/adapter/gateway/gh translates a service's own
// spelling into these values before anything reaches this package.
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

// IsPR reports whether this reference names a pull request. Views ask the
// reference rather than comparing Kind themselves: the comparison is the
// closest thing in this package to the API's own spelling, so it belongs
// inside the domain type rather than scattered across views.
func (r ItemRef) IsPR() bool { return r.Kind == ItemPR }
