// Package usecase decides what calls an operation on the GitHub layer takes,
// and in what order.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
)

type itemFetcher interface {
	GetPR(ctx context.Context, repo string, number int) (domain.PR, error)
	GetIssue(ctx context.Context, repo string, number int) (domain.Issue, error)
}

type commenter interface {
	AddPRComment(repo string, number int, body string) error
	AddIssueComment(repo string, number int, body string) error
}

type stateChanger interface {
	ClosePR(repo string, number int) error
	ReopenPR(repo string, number int) error
	CloseIssue(repo string, number int) error
	ReopenIssue(repo string, number int) error
}

type labelEditor interface {
	EditPRLabels(repo string, number int, add, remove []string) error
	EditIssueLabels(repo string, number int, add, remove []string) error
}

type assigneeEditor interface {
	EditPRAssignees(repo string, number int, add, remove []string) error
	EditIssueAssignees(repo string, number int, add, remove []string) error
}

type lister interface {
	ListPRs(ctx context.Context, repo string) ([]domain.PR, error)
	ListIssues(ctx context.Context, repo string) ([]domain.Issue, error)
	RepoName(ctx context.Context) (string, error)
	ListLabels(ctx context.Context, repo string) ([]domain.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

// crossRepoLister is what an operation that cannot name a single repository
// takes: unlike lister's operations, none of these are "the contents of one
// named repository".
type crossRepoLister interface {
	ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error)
	RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error)
	SearchItems(ctx context.Context, query string) ([]domain.WorkItem, error)
}

// repoFinder is what the add dialog offers: candidates while it is typed
// into, and the repositories a first run can be seeded from.
type repoFinder interface {
	SearchRepos(ctx context.Context, query string, limit int) ([]domain.RepoCandidate, error)
	ListOwnRepos(ctx context.Context, owner string, limit int) ([]domain.RepoCandidate, error)
	ListOrgs(ctx context.Context) ([]string, error)
}

// repoStore is where the sidebar's list survives a restart.
type repoStore interface {
	SaveRepositories(repos []string) error
}

// queryStore is where the Search tab's saved queries survive a restart.
type queryStore interface {
	SaveQueries(queries []domain.SavedQuery) error
}

// settingsStore is the settings file: it satisfies repoStore and queryStore
// both, which is what New's caller (cmd/octoscope's datasource.Store) writes.
type settingsStore interface {
	repoStore
	queryStore
}

type reviewFetcher interface {
	PRDiff(ctx context.Context, repo string, number int) ([]domain.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error)
}

type reviewer interface {
	StartReview(pullRequestID string) (string, error)
	AddReviewThread(reviewID string, c domain.PendingComment) error
	SubmitReview(reviewID string, event domain.ReviewEvent, body string) error
	SubmitNewReview(pullRequestID string, event domain.ReviewEvent, body string) error
	DiscardReview(reviewID string) error
}

type opener interface {
	OpenWeb(url string) error
}

type checksFetcher interface {
	PRChecks(ctx context.Context, repo string, number int) (domain.Checks, error)
	JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]domain.LogLine, error)
	RerunWorkflow(ctx context.Context, repo string, runID int64, scope domain.RerunScope) error
}

type merger interface {
	PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error)
	MergePR(pullRequestID string, method domain.MergeMethod) error
	EnableAutoMerge(pullRequestID string, method domain.MergeMethod) error
	DisableAutoMerge(pullRequestID string) error
}

type source interface {
	itemFetcher
	commenter
	stateChanger
	labelEditor
	assigneeEditor
	lister
	crossRepoLister
	repoFinder
	reviewFetcher
	reviewer
	opener
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
	crossRepo  crossRepoLister
	repos      repoFinder
	repoStore  repoStore
	queryStore queryStore
	reviewInfo reviewFetcher
	reviews    reviewer
	web        opener
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
		crossRepo:  src,
		repos:      src,
		repoStore:  store,
		queryStore: store,
		reviewInfo: src,
		reviews:    src,
		web:        src,
		checks:     src,
		merges:     src,
	}
}

// Item is where a pull request and an issue meet: the fields GitHub gives
// both (.claude/rules/architecture.md).
type Item struct {
	Kind      domain.ItemKind
	Number    int
	Title     string
	Author    domain.Author
	State     domain.ItemState
	Body      string
	URL       string
	Labels    []domain.Label
	Assignees []domain.Author
	Comments  []domain.Comment
	UpdatedAt time.Time

	// PR is set only when Kind is ItemPR.
	PR *domain.PR
}

// GetItem fetches whichever of the two the reference names.
func (u *Usecase) GetItem(ctx context.Context, ref domain.ItemRef) (Item, error) {
	if ref.Kind == domain.ItemPR {
		pr, err := u.items.GetPR(ctx, ref.Repo, ref.Number)
		if err != nil {
			return Item{}, fmt.Errorf("get pr: %w", err)
		}
		return Item{
			Kind: domain.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
			State: pr.State, Body: pr.Body, URL: pr.URL, Labels: pr.Labels,
			Assignees: pr.Assignees, Comments: pr.Comments, UpdatedAt: pr.UpdatedAt,
			PR: &pr,
		}, nil
	}
	issue, err := u.items.GetIssue(ctx, ref.Repo, ref.Number)
	if err != nil {
		return Item{}, fmt.Errorf("get issue: %w", err)
	}
	return Item{
		Kind: domain.ItemIssue, Number: issue.Number, Title: issue.Title, Author: issue.Author,
		State: issue.State, Body: issue.Body, URL: issue.URL, Labels: issue.Labels,
		Assignees: issue.Assignees, Comments: issue.Comments, UpdatedAt: issue.UpdatedAt,
	}, nil
}

func (u *Usecase) AddComment(ref domain.ItemRef, body string) error {
	if ref.Kind == domain.ItemPR {
		return u.comments.AddPRComment(ref.Repo, ref.Number, body)
	}
	return u.comments.AddIssueComment(ref.Repo, ref.Number, body)
}

// SetState closes the item when closing is true and reopens it otherwise.
func (u *Usecase) SetState(ref domain.ItemRef, closing bool) error {
	switch {
	case ref.Kind == domain.ItemPR && closing:
		return u.states.ClosePR(ref.Repo, ref.Number)
	case ref.Kind == domain.ItemPR:
		return u.states.ReopenPR(ref.Repo, ref.Number)
	case closing:
		return u.states.CloseIssue(ref.Repo, ref.Number)
	default:
		return u.states.ReopenIssue(ref.Repo, ref.Number)
	}
}

func (u *Usecase) EditLabels(ref domain.ItemRef, add, remove []string) error {
	if ref.Kind == domain.ItemPR {
		return u.labels.EditPRLabels(ref.Repo, ref.Number, add, remove)
	}
	return u.labels.EditIssueLabels(ref.Repo, ref.Number, add, remove)
}

func (u *Usecase) EditAssignees(ref domain.ItemRef, add, remove []string) error {
	if ref.Kind == domain.ItemPR {
		return u.assignees.EditPRAssignees(ref.Repo, ref.Number, add, remove)
	}
	return u.assignees.EditIssueAssignees(ref.Repo, ref.Number, add, remove)
}

func (u *Usecase) ListWorkSection(ctx context.Context, s domain.WorkSection) ([]domain.WorkItem, error) {
	return u.crossRepo.ListWorkSection(ctx, s)
}

func (u *Usecase) RepoCounts(ctx context.Context, repos []string) ([]domain.RepoCount, error) {
	return u.crossRepo.RepoCounts(ctx, repos)
}

func (u *Usecase) ListPRs(ctx context.Context, repo string) ([]domain.PR, error) {
	return u.lists.ListPRs(ctx, repo)
}

func (u *Usecase) ListIssues(ctx context.Context, repo string) ([]domain.Issue, error) {
	return u.lists.ListIssues(ctx, repo)
}

func (u *Usecase) RepoName(ctx context.Context) (string, error) { return u.lists.RepoName(ctx) }

func (u *Usecase) ListLabels(ctx context.Context, repo string) ([]domain.Label, error) {
	return u.lists.ListLabels(ctx, repo)
}

func (u *Usecase) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	return u.lists.ListAssignees(ctx, repo)
}

func (u *Usecase) PRDiff(ctx context.Context, repo string, number int) ([]domain.FileDiff, error) {
	return u.reviewInfo.PRDiff(ctx, repo, number)
}

func (u *Usecase) PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error) {
	return u.reviewInfo.PRReviewContext(ctx, repo, number)
}

func (u *Usecase) DiscardReview(reviewID string) error {
	return u.reviews.DiscardReview(reviewID)
}

func (u *Usecase) OpenWeb(url string) error { return u.web.OpenWeb(url) }

func (u *Usecase) PRChecks(ctx context.Context, repo string, number int) (domain.Checks, error) {
	return u.checks.PRChecks(ctx, repo, number)
}

func (u *Usecase) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]domain.LogLine, error) {
	return u.checks.JobLog(ctx, repo, jobID, failedOnly)
}

func (u *Usecase) RerunWorkflow(ctx context.Context, repo string, runID int64, scope domain.RerunScope) error {
	return u.checks.RerunWorkflow(ctx, repo, runID, scope)
}

func (u *Usecase) PRMergeContext(ctx context.Context, repo string, number int) (domain.MergeContext, error) {
	return u.merges.PRMergeContext(ctx, repo, number)
}

func (u *Usecase) MergePR(pullRequestID string, method domain.MergeMethod) error {
	return u.merges.MergePR(pullRequestID, method)
}

func (u *Usecase) EnableAutoMerge(pullRequestID string, method domain.MergeMethod) error {
	return u.merges.EnableAutoMerge(pullRequestID, method)
}

func (u *Usecase) DisableAutoMerge(pullRequestID string) error {
	return u.merges.DisableAutoMerge(pullRequestID)
}
