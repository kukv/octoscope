package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

func TestIsPRAnswersFromTheKindAlone(t *testing.T) {
	t.Parallel()

	pr := domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 1}
	if !pr.IsPR() {
		t.Error("a ref whose kind is ItemPR does not say it is a pull request")
	}

	issue := domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 1}
	if issue.IsPR() {
		t.Error("a ref whose kind is ItemIssue says it is a pull request")
	}
}

func TestIsIssueAnswersFromTheKindAlone(t *testing.T) {
	t.Parallel()

	issue := domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 1}
	if !issue.IsIssue() {
		t.Error("a ref whose kind is ItemIssue does not say it is an issue")
	}

	pr := domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 1}
	if pr.IsIssue() {
		t.Error("a ref whose kind is ItemPR says it is an issue")
	}
}
