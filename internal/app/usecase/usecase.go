// Package usecase decides what calls an operation on the GitHub layer takes,
// and in what order.
package usecase

// settingsStore is the settings file: it satisfies repoStore and queryStore
// both, which is what New's caller (cmd/octoscope's datasource.Store) writes.
type settingsStore interface {
	repoStore
	queryStore
}

type source interface {
	itemFetcher
	commenter
	stateChanger
	labelEditor
	assigneeEditor
	lister
	viewerFetcher
	crossRepoLister
	repoFinder
	reviewFetcher
	reviewer
	checksFetcher
	merger
}

// Usecase holds the backend every view talks to.
type Usecase struct {
	items      itemFetcher
	comments   commenter
	states     stateChanger
	labels     labelEditor
	assignees  assigneeEditor
	lists      lister
	viewer     viewerFetcher
	crossRepo  crossRepoLister
	repos      repoFinder
	repoStore  repoStore
	queryStore queryStore
	reviewInfo reviewFetcher
	reviews    reviewer
	checks     checksFetcher
	merges     merger
}

// New wires a Usecase to one backend and the settings file its repository
// list and saved queries are written to.
func New(src source, store settingsStore) *Usecase {
	return &Usecase{
		items:      src,
		comments:   src,
		states:     src,
		labels:     src,
		assignees:  src,
		lists:      src,
		viewer:     src,
		crossRepo:  src,
		repos:      src,
		repoStore:  store,
		queryStore: store,
		reviewInfo: src,
		reviews:    src,
		checks:     src,
		merges:     src,
	}
}
