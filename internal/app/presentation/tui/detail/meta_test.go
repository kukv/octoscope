package detail

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/usecase"
	"github.com/kukv/octoscope/internal/i18n"
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

// spacedLabelItem carries the two facts that hold a space of their own: a
// label whose name is three words, and a branch pair with arrows around it.
func spacedLabelItem() usecase.Item {
	it := fullPRItem()
	it.Labels = []domain.Label{{Name: "good first issue", Color: "d73a4a"}}
	it.PR.Labels = it.Labels
	return it
}

// TestTheInlineMetaKeepsAFactOnOneLine is why the paragraph is filled a fact
// at a time. A fact broken over two lines reads as two facts, and a break
// inside a label's badge loses the padding that makes it a badge.
func TestTheInlineMetaKeepsAFactOnOneLine(t *testing.T) {
	const narrow = 44

	lines := metaInlineLines(metaRows(fullRef(), spacedLabelItem()), narrow)

	for _, fact := range []string{
		"kukv/octoscope #12", "good first issue", "feat/x → main",
		"assignees @alice", "Sep 6, 2026 12:00",
	} {
		if !slices.ContainsFunc(lines, func(l string) bool {
			return strings.Contains(ansi.Strip(l), fact)
		}) {
			t.Errorf("%q was split across lines:\n%s", fact, ansi.Strip(strings.Join(lines, "\n")))
		}
	}
}

// TestTheInlineMetaFitsANarrowWidth includes a width under which no fact fits
// whole: the paragraph may not put a fact on a line of its own and call it
// fitted.
func TestTheInlineMetaFitsANarrowWidth(t *testing.T) {
	for _, narrow := range []int{44, 12} {
		for _, l := range metaInlineLines(metaRows(fullRef(), spacedLabelItem()), narrow) {
			if w := ansi.StringWidth(l); w > narrow {
				t.Errorf("at %d columns the line %q is %d wide", narrow, ansi.Strip(l), w)
			}
		}
	}
}

// TestEveryMetaLabelFitsItsColumn guards the constant against a catalog: a
// label wider than the column would be clipped, and layout.Pad clips to one
// column short of what it is given.
func TestEveryMetaLabelFitsItsColumn(t *testing.T) {
	ids := []string{
		"detail.meta.repo", "detail.meta.author", "detail.meta.state",
		"detail.meta.review", "detail.meta.checks", "detail.meta.branch",
		"detail.meta.changes", "detail.meta.assignees", "detail.meta.labels",
		"detail.meta.updated",
	}
	t.Cleanup(func() { i18n.SetLanguage(language.English) })
	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, id := range ids {
			if w := ansi.StringWidth(i18n.T(id)); w > metaLabelWidth-1 {
				t.Errorf("%s in %s is %d columns wide, want at most %d",
					id, lang, w, metaLabelWidth-1)
			}
		}
	}
}

// valueOf is what the pane draws beside a label. The rows are searched by
// label because their order is what the other tests are for.
func valueOf(rows []metaRow, label string) string {
	for _, r := range rows {
		if r.label == label {
			return ansi.Strip(r.value)
		}
	}
	return ""
}

// TestADraftSaysSoInItsState covers the suffix the state word alone leaves
// out: a draft is open, and "open" is what every other open pull request says
// too. An issue is never a draft, so its state carries nothing extra.
func TestADraftSaysSoInItsState(t *testing.T) {
	draft := fullPRItem()
	draft.PR.IsDraft = true

	got := valueOf(metaRows(fullRef(), draft), i18n.T("detail.meta.state"))
	want := i18n.T("state.open") + i18n.T("state.draft_suffix")
	if got != want {
		t.Errorf("the draft's state = %q, want %q", got, want)
	}

	issue := usecase.Item{
		Kind: domain.ItemIssue, Number: 7, Title: "an issue",
		Author: domain.Author{Login: "kukv"}, State: domain.StateOpen, UpdatedAt: metaAt(),
	}
	ref := domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 7}
	if got := valueOf(metaRows(ref, issue), i18n.T("detail.meta.state")); got != i18n.T("state.open") {
		t.Errorf("the issue's state = %q, want %q", got, i18n.T("state.open"))
	}
}
