// Package usecase is where an operation that takes more than one call to the
// GitHub layer, or that picks a call by the kind of item, is decided. The
// views above it say what they want done; which requests that takes, and in
// what order, is not their business.
package usecase

import (
	"context"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

// The source interfaces are split by what each operation needs, so a test can
// build a fake with only the methods that operation calls
// (.claude/rules/architecture.md).

type itemFetcher interface {
	GetPR(ctx context.Context, repo string, number int) (gh.PR, error)
	GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error)
}

// The write side is split four ways rather than kept as one itemWriter: ten
// methods on one declaration is over the limit the rules set, and GitHub
// having a separate endpoint per kind is not a reason for this package to
// have one big interface.

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

// reviewFetcher is the read half of the review API. It is split out from
// reviewer so a fake for one does not have to stub the other.
type reviewFetcher interface {
	PRDiff(ctx context.Context, repo string, number int) ([]gh.FileDiff, error)
	PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error)
}

// reviewer is the write half of the review API: starting a pending review,
// adding threads to it, and submitting or discarding it.
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

// source is the whole of what a backend has to provide. It is a composition
// of the interfaces above rather than one long list, so that a test can build
// a fake for just the half it exercises (see usecase_test.go, which is in
// this package for exactly that reason).
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

// Usecase holds one backend, split across the fields by what each operation
// needs. The split is what keeps any single interface declaration small.
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

// New wires a Usecase to one backend. cmd/octoscope passes a *cli.Client;
// the compiler checks there that it still has every method, which is the
// whole point of naming the union.
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

// Item is where a pull request and an issue meet. The detail view draws one
// screen for both, and the two GitHub types do not unify -- so this is the
// shape that screen reads. It is not a DTO: nothing here is a second spelling
// of a domain field, and PR is the pull request itself, not a copy of it.
//
// A field belongs on Item only when GitHub gives both a pull request and an
// issue their own. Anything only a pull request has is read through PR, not
// copied up to here. Adding a field because one screen wants to draw it is
// how this turns into a DTO, and a DTO is what a screen's needs leaking a
// layer down looks like.
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

	// PR is set only when Kind is ItemPR. It carries what an issue has no
	// equivalent of: the branches, the size of the change, the checks and
	// the review decision.
	PR *gh.PR
}

// GetItem fetches whichever of the two the reference names. Choosing between
// them is what ItemRef.Kind is for, and no view has to do it.
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
// Whether either is possible is the view's question -- a merged pull request
// is neither -- and it asks before calling.
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

// The rest delegate one for one. They are here so that every view asks the
// same package for everything: an operation that grows a second call later
// grows it in one place, and no view has to change which package it talks to.

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
