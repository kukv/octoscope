package repo

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/drawer"
	"github.com/kukv/octoscope/internal/i18n"
)

func next(m Model, k string) Model {
	m, _ = m.Update(key(k))
	return m
}

// TestARowShowsTheStateTheNumberAndTheAge pins the table's fixed fields
// The title takes whatever the others leave.
func TestARowShowsTheStateTheNumberAndTheAge(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		pr   domain.Item
		want []string
	}{
		{"draft", domain.Item{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: 1},
			UpdatedAt: now.Add(-30 * time.Second),
			Change: &domain.Change{
				IsDraft: true,
			},
		}, []string{"◌", "#1", "now"}},
		{"approved", domain.Item{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: 2},
			UpdatedAt: now.Add(-5 * time.Minute),
			Change: &domain.Change{
				Review: domain.ReviewApproved,
			},
		}, []string{"✓", "#2", "5m ago"}},
		{"changes requested", domain.Item{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: 3},
			UpdatedAt: now.Add(-3 * time.Hour),
			Change: &domain.Change{
				Review: domain.ReviewChangesRequested,
			},
		}, []string{"×", "#3", "3h ago"}},
		{"review required", domain.Item{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: 4},
			UpdatedAt: now.Add(-49 * time.Hour),
			Change: &domain.Change{
				Review: domain.ReviewRequired,
			},
		}, []string{"•", "#4", "2d ago"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := sized(loadedModel(&fakeSource{prs: []domain.Item{
				c.pr,
			}}), 120)
			m.fetchedAt = [2]time.Time{now, now}
			got := ansi.Strip(m.row(0))
			for _, want := range c.want {
				if !strings.Contains(got, want) {
					t.Errorf("row = %q, want to contain %q", got, want)
				}
			}
		})
	}
}

// TestTheColumnsLineUpDownThePage is what the fixed widths are for: the eye
// runs down one column, and a Japanese title must not push what follows it
// sideways.
func TestTheColumnsLineUpDownThePage(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	prs := []domain.Item{
		{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: 1},
			Title:     "short",
			UpdatedAt: now.Add(-time.Hour),
			Change:    &domain.Change{},
		},
		{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: 22},
			Title:     "レンダリングのパイプラインをまるごと置き換える",
			UpdatedAt: now.Add(-time.Hour),
			Change:    &domain.Change{},
		},
		{
			Ref:       domain.ItemRef{Kind: domain.ItemPR, Number: 333},
			Title:     "a middling sort of title",
			UpdatedAt: now.Add(-time.Hour),
			Change:    &domain.Change{},
		},
	}
	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{80, 120, 160} {
			m := sized(loadedModel(&fakeSource{prs: prs}), width)
			m.fetchedAt = [2]time.Time{now, now}

			for i := range prs {
				line := ansi.Strip(m.row(i))
				if got := ansi.StringWidth(line); got != width {
					t.Errorf("lang %s width %d: row %d is %d columns: %q", lang, width, i, got, line)
				}
				// The age ends flush with the right edge in every row.
				if !strings.HasSuffix(line, i18n.RelTime(now, prs[i].UpdatedAt)) {
					t.Errorf("lang %s width %d: row %d does not end with its age: %q",
						lang, width, i, line)
				}
			}
		}
	}
}

// TestTheDrawerIsAlwaysTheSameHeight keeps the table still: the block is
// drawn under it, and one that grew with the selection would move the key bar.
func TestTheDrawerIsAlwaysTheSameHeight(t *testing.T) {
	f := &fakeSource{
		prs: []domain.Item{
			{
				Ref:   domain.ItemRef{Kind: domain.ItemPR, Number: 1},
				Title: "with checks",
				Change: &domain.Change{
					Head:      "a",
					Base:      "main",
					Additions: 2,
					Deletions: 1,
					Checks: domain.Checks{
						Total: 2, Passed: 2, State: domain.CheckSuccess,
						Runs: []domain.CheckRun{{Name: "lint"}, {Name: "test"}},
					},
				},
			},
			{
				Ref:    domain.ItemRef{Kind: domain.ItemPR, Number: 2},
				Title:  "without",
				Change: &domain.Change{},
			},
		},
		issues: []domain.Item{
			{
				Ref:    domain.ItemRef{Kind: domain.ItemIssue, Number: 9},
				Title:  "an issue",
				Author: domain.Author{Login: "bob"},
			},
		},
	}
	m := sized(loadedModel(f), 120)
	for name, model := range map[string]Model{
		"a PR with checks": m,
		"a PR without":     next(m, "j"),
	} {
		if got := len(model.drawerLines()); got != drawer.Height {
			t.Errorf("%s: the drawer is %d lines, want %d", name, got, drawer.Height)
		}
	}
}

// TestTheDrawerNamesTheBranchesAndTheSizeOfTheChange is the line the mockup
// puts under the table.
func TestTheDrawerNamesTheBranchesAndTheSizeOfTheChange(t *testing.T) {
	f := &fakeSource{prs: []domain.Item{
		{
			Ref:    domain.ItemRef{Kind: domain.ItemPR, Number: 1},
			Title:  "a change",
			Author: domain.Author{Login: "kukv"},
			Change: &domain.Change{
				Head:      "feat/graph",
				Base:      "main",
				Additions: 218,
				Deletions: 31,
				Checks: domain.Checks{
					Total: 1, Passed: 1, State: domain.CheckSuccess,
					Runs: []domain.CheckRun{{Name: "lint", State: domain.CheckSuccess}},
				},
			},
		},
	}}
	got := ansi.Strip(strings.Join(sized(loadedModel(f), 120).drawerLines(), "\n"))
	for _, want := range []string{"@kukv", "feat/graph", "main", "+218", "−31", "lint"} {
		if !strings.Contains(got, want) {
			t.Errorf("the drawer is missing %q:\n%s", want, got)
		}
	}
}

// TestTheListFitsTheTerminal is the same rule the board follows: everything
// drawn has to be on screen, so a repository with a hundred pull requests
// scrolls rather than pushing the key bar off the bottom.
func TestTheListFitsTheTerminal(t *testing.T) {
	var prs []domain.Item
	for i := range 60 {
		prs = append(prs, domain.Item{
			Ref:    domain.ItemRef{Kind: domain.ItemPR, Number: i + 1},
			Title:  "a pull request",
			Change: &domain.Change{},
		})
	}
	m := loadedModel(&fakeSource{prs: prs})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})

	out := m.View()
	if got := len(strings.Split(out, "\n")); got > 24 {
		t.Errorf("the list drew %d lines into a terminal 24 high", got)
	}
	if !strings.Contains(ansi.Strip(out), ansi.Strip(m.keyBar())) {
		t.Errorf("the key bar was pushed off the screen:\n%s", ansi.Strip(out))
	}

	// The cursor stays on screen as it travels past the last visible row.
	for range m.visibleRows() + 3 {
		m, _ = m.Update(key("j"))
	}
	if !strings.Contains(ansi.Strip(m.View()), fmt.Sprintf("#%d", m.cursors[tabPRs]+1)) {
		t.Errorf("the selected row is not on screen:\n%s", ansi.Strip(m.View()))
	}
}

// TestTheKeyBarSitsOnTheLastRow is the complaint this change answers: with
// two open pull requests the bar used to sit halfway up the terminal, and
// switching between here and the board moved the hints under the user's eyes.
func TestTheKeyBarSitsOnTheLastRow(t *testing.T) {
	for _, count := range []int{0, 2, 60} {
		prs := make([]domain.Item, count)
		for i := range prs {
			prs[i] = domain.Item{
				Ref:    domain.ItemRef{Kind: domain.ItemPR, Number: i + 1},
				Title:  "a pull request",
				Change: &domain.Change{},
			}
		}
		m := currentModel(&fakeSource{prs: prs}, 120)

		if got := len(strings.Split(m.View(), "\n")); got != 40 {
			t.Errorf("%d pull requests: the view is %d rows, want 40", count, got)
		}
	}
}

// TestTheDrawerSitsAboveTheKeyBar is the other half of that: the block is
// anchored to the bottom the way the board's is, so neither it nor the table
// above it moves as rows arrive.
func TestTheDrawerSitsAboveTheKeyBar(t *testing.T) {
	const height = 40
	want := height - footerHeight - drawer.Height

	for _, count := range []int{2, 20} {
		m := currentModel(&fakeSource{prs: somePRs(count)}, 120)

		rule := strings.TrimRight(ansi.Strip(m.drawerLines()[0]), " ")
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		got := -1
		for i, line := range lines {
			if strings.TrimRight(line, " ") == rule {
				got = i
				break
			}
		}
		if got != want {
			t.Errorf("%d pull requests: the drawer starts on row %d, want %d:\n%s",
				count, got, want, strings.Join(lines, "\n"))
		}
	}
}

// TestTheDrawerIsTheOneTheBoardDraws is the point of the change: the same
// lines, in the same colours, under both tabs.
func TestTheDrawerIsTheOneTheBoardDraws(t *testing.T) {
	f := &fakeSource{prs: []domain.Item{
		{
			Ref:    domain.ItemRef{Kind: domain.ItemPR, Number: 1},
			Title:  "a change",
			Author: domain.Author{Login: "kukv"},
			Body:   "What this changes and why.",
			Change: &domain.Change{
				Head:      "feat/graph",
				Base:      "main",
				Additions: 218,
				Deletions: 31,
				Checks: domain.Checks{
					Total: 1, Passed: 1, State: domain.CheckSuccess,
					Runs: []domain.CheckRun{{Name: "lint", State: domain.CheckSuccess}},
				},
			},
		},
	}}
	m := currentModel(f, 120)

	it, ok := m.selectedItem()
	if !ok {
		t.Fatal("nothing is selected in a list with one pull request")
	}
	want := drawer.Render(it, 120)
	lines := strings.Split(m.View(), "\n")
	got := lines[len(lines)-footerHeight-drawer.Height : len(lines)-footerHeight]

	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the block under the list is not the board's drawer:\n--- got ---\n%s\n--- want ---\n%s",
			ansi.Strip(strings.Join(got, "\n")), ansi.Strip(strings.Join(want, "\n")))
	}
}

// TestTheDrawerPreviewsTheBodyAsText is the last piece of "the same drawer on
// both tabs". The board previews plain text, so this tab must too, or a pull
// request template's comment markers and headings would fill three lines here
// and read as prose there.
func TestTheDrawerPreviewsTheBodyAsText(t *testing.T) {
	f := &fakeSource{prs: []domain.Item{
		{
			Ref:      domain.ItemRef{Kind: domain.ItemPR, Number: 1},
			Title:    "a change",
			Body:     "<!-- tell us why -->\n## Why\nBecause it was broken.",
			BodyText: "Why Because it was broken.",
			Change:   &domain.Change{},
		},
	}}
	got := ansi.Strip(strings.Join(currentModel(f, 120).drawerLines(), "\n"))

	if !strings.Contains(got, "Because it was broken.") {
		t.Errorf("the drawer does not preview the body:\n%s", got)
	}
	for _, markup := range []string{"<!--", "## "} {
		if strings.Contains(got, markup) {
			t.Errorf("the preview carries the markdown %q:\n%s", markup, got)
		}
	}
}

// TestTheDrawerSpansTheWholeWidth is what "the same as the board" means: the
// rule runs under the sidebar too, rather than starting at the table's edge.
func TestTheDrawerSpansTheWholeWidth(t *testing.T) {
	m := currentModel(&fakeSource{prs: somePRs(3)}, 120)

	rule := ansi.Strip(m.drawerLines()[0])
	if got := ansi.StringWidth(rule); got != 120 {
		t.Errorf("the rule is %d columns, want 120: %q", got, rule)
	}
	if !strings.Contains(ansi.Strip(m.View()), rule) {
		t.Errorf("the full-width rule is not on screen:\n%s", ansi.Strip(m.View()))
	}
}

// TestTheDrawerFoldsAwayWhenNarrow is the third step of the spec's
// degradation: under a hundred columns there is no room for two panes side by
// side, so the block goes and the table takes the rows back.
func TestTheDrawerFoldsAwayWhenNarrow(t *testing.T) {
	f := &fakeSource{prs: somePRs(60)}
	wide := currentModel(f, 120)
	narrow := currentModel(f, 80)

	if narrow.drawerShown() {
		t.Fatal("the drawer is still drawn at eighty columns")
	}
	if got, want := narrow.visibleRows(), wide.visibleRows()+drawer.Height; got != want {
		t.Errorf("the narrow table draws %d rows, want %d", got, want)
	}
	if got := len(strings.Split(narrow.View(), "\n")); got != 40 {
		t.Errorf("the narrow view is %d rows, want 40", got)
	}
}

// somePRs is a list long enough to scroll, with branches so the drawer has
// something to say about whichever is selected.
func somePRs(n int) []domain.Item {
	prs := make([]domain.Item, n)
	for i := range prs {
		prs[i] = domain.Item{
			Ref:   domain.ItemRef{Kind: domain.ItemPR, Number: i + 1},
			Title: "a pull request",
			Change: &domain.Change{
				Head: "topic",
				Base: "main",
			},
		}
	}
	return prs
}
