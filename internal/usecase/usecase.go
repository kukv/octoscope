// Package usecase decides what calls an operation on the GitHub layer takes,
// and in what order.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

type itemFetcher interface {
	GetPR(ctx context.Context, repo string, number int) (gh.PR, error)
	GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error)
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
	ListWork(ctx context.Context) (gh.Work, error)
	ListPRs(ctx context.Context) ([]gh.PR, error)
	ListIssues(ctx context.Context) ([]gh.Issue, error)
	RepoName(ctx context.Context) (string, error)
	ListLabels(ctx context.Context, repo string) ([]gh.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

type reviewFetcher interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
}

type reviewer interface {
	StartReview(pullRequestID string) (string, error)
	AddReviewThread(reviewID string, c gh.PendingComment) error
	SubmitReview(reviewID string, event gh.ReviewEvent, body string) error
	SubmitNewReview(pullRequestID string, event gh.ReviewEvent, body string) error
	DiscardReview(reviewID string) error
}

type opener interface {
	OpenWeb(url string) error
}

type source interface {
	itemFetcher
	commenter
	stateChanger
	labelEditor
	assigneeEditor
	lister
	reviewFetcher
	reviewer
	opener
}

// Usecase holds the backend every view talks to.
type Usecase struct {
	items      itemFetcher
	comments   commenter
	states     stateChanger
	labels     labelEditor
	assignees  assigneeEditor
	lists      lister
	reviewInfo reviewFetcher
	reviews    reviewer
	web        opener
}

// New wires a Usecase to one backend.
func New(src source) *Usecase {
	return &Usecase{
		items:      src,
		comments:   src,
		states:     src,
		labels:     src,
		assignees:  src,
		lists:      src,
		reviewInfo: src,
		reviews:    src,
		web:        src,
	}
}

// Item is where a pull request and an issue meet: the fields GitHub gives
// both (.claude/rules/architecture.md).
type Item struct {
	Kind      gh.ItemKind
	Number    int
	Title     string
	Author    gh.Author
	State     gh.ItemState
	Body      string
	URL       string
	Labels    []gh.Label
	Assignees []gh.Author
	Comments  []gh.Comment
	UpdatedAt time.Time

	// PR is set only when Kind is ItemPR.
	PR *gh.PR
}

// GetItem fetches whichever of the two the reference names.
func (u *Usecase) GetItem(ctx context.Context, ref gh.ItemRef) (Item, error) {
	if ref.Kind == gh.ItemPR {
		pr, err := u.items.GetPR(ctx, ref.Repo, ref.Number)
		if err != nil {
			return Item{}, fmt.Errorf("get pr: %w", err)
		}
		return Item{
			Kind: gh.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
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
		Kind: gh.ItemIssue, Number: issue.Number, Title: issue.Title, Author: issue.Author,
		State: issue.State, Body: issue.Body, URL: issue.URL, Labels: issue.Labels,
		Assignees: issue.Assignees, Comments: issue.Comments, UpdatedAt: issue.UpdatedAt,
	}, nil
}

func (u *Usecase) AddComment(ref gh.ItemRef, body string) error {
	if ref.Kind == gh.ItemPR {
		return u.comments.AddPRComment(ref.Repo, ref.Number, body)
	}
	return u.comments.AddIssueComment(ref.Repo, ref.Number, body)
}

// SetState closes the item when closing is true and reopens it otherwise.
func (u *Usecase) SetState(ref gh.ItemRef, closing bool) error {
	switch {
	case ref.Kind == gh.ItemPR && closing:
		return u.states.ClosePR(ref.Repo, ref.Number)
	case ref.Kind == gh.ItemPR:
		return u.states.ReopenPR(ref.Repo, ref.Number)
	case closing:
		return u.states.CloseIssue(ref.Repo, ref.Number)
	default:
		return u.states.ReopenIssue(ref.Repo, ref.Number)
	}
}

func (u *Usecase) EditLabels(ref gh.ItemRef, add, remove []string) error {
	if ref.Kind == gh.ItemPR {
		return u.labels.EditPRLabels(ref.Repo, ref.Number, add, remove)
	}
	return u.labels.EditIssueLabels(ref.Repo, ref.Number, add, remove)
}

func (u *Usecase) EditAssignees(ref gh.ItemRef, add, remove []string) error {
	if ref.Kind == gh.ItemPR {
		return u.assignees.EditPRAssignees(ref.Repo, ref.Number, add, remove)
	}
	return u.assignees.EditIssueAssignees(ref.Repo, ref.Number, add, remove)
}

func (u *Usecase) ListWork(ctx context.Context) (gh.Work, error) { return u.lists.ListWork(ctx) }

func (u *Usecase) ListPRs(ctx context.Context) ([]gh.PR, error) { return u.lists.ListPRs(ctx) }

func (u *Usecase) ListIssues(ctx context.Context) ([]gh.Issue, error) {
	return u.lists.ListIssues(ctx)
}

func (u *Usecase) RepoName(ctx context.Context) (string, error) { return u.lists.RepoName(ctx) }

func (u *Usecase) ListLabels(ctx context.Context, repo string) ([]gh.Label, error) {
	return u.lists.ListLabels(ctx, repo)
}

func (u *Usecase) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	return u.lists.ListAssignees(ctx, repo)
}

func (u *Usecase) PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error) {
	return u.reviewInfo.PRDiff(ctx, repo, number)
}

func (u *Usecase) PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error) {
	return u.reviewInfo.PRReviewContext(ctx, repo, number)
}

func (u *Usecase) DiscardReview(reviewID string) error {
	return u.reviews.DiscardReview(reviewID)
}

func (u *Usecase) OpenWeb(url string) error { return u.web.OpenWeb(url) }
