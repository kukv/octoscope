package drawer_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/drawer"
)

// item is a pull request with something to say in every part of the drawer.
func item() domain.WorkItem {
	return domain.WorkItem{
		Ref:       domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 42},
		Title:     "fix: the thing that was broken",
		Body:      "The first paragraph of the body.\n\nAnd a second one.",
		Author:    "kukv",
		Head:      "fix/the-thing",
		Base:      "main",
		Additions: 12,
		Deletions: 3,
		Labels:    []domain.Label{{Name: "bug", Color: "d73a4a"}},
		Checks: domain.Checks{
			Total: 2, Passed: 2, State: domain.CheckSuccess,
			Runs: []domain.CheckRun{{Name: "lint"}, {Name: "test"}},
		},
	}
}

// TestRenderIsTheFixedHeight is what the caller budgets against: the drawer
// sits under a list whose length depends on its contents, and one that
// changed height would move the key bar under the user's eyes.
func TestRenderIsTheFixedHeight(t *testing.T) {
	long := item()
	long.Body = strings.Repeat("a long body that wraps many times over. ", 40)
	many := item()
	many.Checks = domain.Checks{Total: 9, Passed: 9, State: domain.CheckSuccess}
	for i := range 9 {
		many.Checks.Runs = append(many.Checks.Runs, domain.CheckRun{Name: string(rune('a' + i))})
	}
	noBody := item()
	noBody.Body = ""
	issue := item()
	issue.Ref.Kind = domain.ItemIssue
	issue.Checks = domain.Checks{}

	for name, it := range map[string]domain.WorkItem{
		"a body that fills it":  long,
		"more checks than fit":  many,
		"nothing in the body":   noBody,
		"an issue, with no CI":  issue,
		"everything it can say": item(),
	} {
		if got := len(drawer.Render(it, 120)); got != drawer.Height {
			t.Errorf("%s: the drawer is %d lines, want %d", name, got, drawer.Height)
		}
	}
}

// TestEmptyIsTheSameHeight covers the no-selection state: an empty column has
// nothing to show, and the rows below it must not move up.
func TestEmptyIsTheSameHeight(t *testing.T) {
	if got := len(drawer.Empty(120)); got != drawer.Height {
		t.Errorf("the empty drawer is %d lines, want %d", got, drawer.Height)
	}
}

// TestRenderFitsTheWidth guards the two-pane split at every width the drawer
// is drawn at. Japanese takes two columns per character, so a title written
// in it costs twice what counting its runes would say.
func TestRenderFitsTheWidth(t *testing.T) {
	ja := item()
	ja.Title = "レンダリングのパイプラインをまるごと置き換える"
	ja.Body = strings.Repeat("日本語の本文がここに続く。", 30)
	ja.Labels = []domain.Label{{Name: "ドキュメント", Color: "0075ca"}}

	for _, w := range []int{drawer.MinColumns, 120, 160} {
		for _, it := range []domain.WorkItem{item(), ja} {
			for i, line := range drawer.Render(it, w) {
				if got := ansi.StringWidth(line); got > w {
					t.Errorf("width %d: line %d is %d columns: %q", w, i, got, line)
				}
			}
		}
	}
}

// TestTheRuleSpansTheWholeWidth is what separates the drawer from what is
// above it: a rule that stopped short would read as a pane's edge.
func TestTheRuleSpansTheWholeWidth(t *testing.T) {
	rule := ansi.Strip(drawer.Render(item(), 120)[0])
	if got := ansi.StringWidth(rule); got != 120 {
		t.Errorf("the rule is %d columns, want 120: %q", got, rule)
	}
}
