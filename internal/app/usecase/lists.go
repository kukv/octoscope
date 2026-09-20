package usecase

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
)

type lister interface {
	RepoName(ctx context.Context) (string, error)
	ListLabels(ctx context.Context, repo string) ([]domain.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

// itemLister is one repository's open items of one kind. The kind is a
// parameter of the query, not a branch: the Repos tab draws pull requests
// and issues in separate panes, so the caller knows which it wants.
type itemLister interface {
	ListItems(ctx context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error)
}

// viewerFetcher names the signed-in user. It is not part of lister: there is
// no repository involved, and nothing is being listed.
type viewerFetcher interface {
	Viewer(ctx context.Context) (string, error)
}

func (u *Usecase) ListItems(ctx context.Context, repo string, kind domain.ItemKind) ([]domain.Item, error) {
	return u.itemLists.ListItems(ctx, repo, kind)
}

func (u *Usecase) RepoName(ctx context.Context) (string, error) { return u.lists.RepoName(ctx) }

// Viewer is the login of the signed-in user.
func (u *Usecase) Viewer(ctx context.Context) (string, error) { return u.viewer.Viewer(ctx) }

func (u *Usecase) ListLabels(ctx context.Context, repo string) ([]domain.Label, error) {
	return u.lists.ListLabels(ctx, repo)
}

func (u *Usecase) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	return u.lists.ListAssignees(ctx, repo)
}
