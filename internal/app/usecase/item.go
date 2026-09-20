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
