package detail

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/usecase"
)

func metaAt() time.Time { return time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC) }

func fullPRItem() usecase.Item {
	pr := domain.PR{
		Number: 12, Title: "a pr", Author: domain.Author{Login: "kukv"},
		State: domain.StateOpen, Review: domain.ReviewApproved, UpdatedAt: metaAt(),
		Labels:    []domain.Label{{Name: "bug", Color: "d73a4a"}},
		Assignees: []domain.Author{{Login: "alice"}},
		Checks:    domain.Checks{Total: 3, Passed: 1, Failed: 1, Running: 1},
		Head:      "feat/x", Base: "main", Additions: 218, Deletions: 31,
	}
	return usecase.Item{
		Kind: domain.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
		State: pr.State, Labels: pr.Labels, Assignees: pr.Assignees,
		UpdatedAt: pr.UpdatedAt, PR: &pr,
	}
}

func fullRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12}
}

// labelsOf is what the row order is checked against: the values carry ANSI,
// the labels are what names the row.
func labelsOf(rows []metaRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.label
	}
	return out
}

func TestAPullRequestGetsEveryRow(t *testing.T) {
	rows := metaRows(fullRef(), fullPRItem())

	want := []string{
		"repository", "author", "state", "review", "checks",
		"branch", "changes", "assignees", "labels", "updated",
	}
	if got := labelsOf(rows); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestAnIssueHasNoPullRequestRows(t *testing.T) {
	it := usecase.Item{
		Kind: domain.ItemIssue, Number: 7, Title: "an issue",
		Author: domain.Author{Login: "kukv"}, State: domain.StateOpen, UpdatedAt: metaAt(),
	}
	ref := domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 7}

	for _, gone := range []string{"review", "checks", "branch", "changes"} {
		for _, label := range labelsOf(metaRows(ref, it)) {
			if label == gone {
				t.Errorf("an issue was given the %q row", gone)
			}
		}
	}
}

// TestAnEmptyValueTakesNoRow keeps the pane from drawing labels with nothing
// after them.
func TestAnEmptyValueTakesNoRow(t *testing.T) {
	pr := domain.PR{
		Number: 3, Author: domain.Author{Login: "kukv"}, State: domain.StateOpen,
		Review: domain.ReviewNone, UpdatedAt: metaAt(),
	}
	it := usecase.Item{
		Kind: domain.ItemPR, Number: pr.Number, Author: pr.Author,
		State: pr.State, UpdatedAt: pr.UpdatedAt, PR: &pr,
	}

	want := []string{"repository", "author", "state", "updated"}
	got := labelsOf(metaRows(fullRef(), it))
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("rows = %v, want %v", got, want)
	}
}

func TestTheMetaPaneFitsItsWidth(t *testing.T) {
	lines := metaPaneLines(metaRows(fullRef(), fullPRItem()), 28)

	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 28 {
			t.Errorf("line %q is %d columns wide, want at most 28", ansi.Strip(l), w)
		}
	}
}

// TestTheInlineMetaNamesTheAssignees covers the one label the single-column
// layout keeps: "@alice" on its own reads as the author.
func TestTheInlineMetaNamesTheAssignees(t *testing.T) {
	lines := metaInlineLines(metaRows(fullRef(), fullPRItem()), 80)

	joined := ansi.Strip(strings.Join(lines, " "))
	if !strings.Contains(joined, "assignees @alice") {
		t.Errorf("the inline meta does not name the assignees:\n%s", joined)
	}
	for _, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line %q is %d columns wide, want at most 80", ansi.Strip(l), w)
		}
	}
}
