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
