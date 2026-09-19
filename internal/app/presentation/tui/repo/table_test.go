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
	"github.com/kukv/octoscope/internal/i18n"
)

// next is the model after one key press.
func next(m Model, k string) Model {
	m, _ = m.Update(key(k))
	return m
}

// TestARowShowsTheStateTheNumberAndTheAge pins the table's fixed fields
// (spec 4.2). The title takes whatever the others leave.
func TestARowShowsTheStateTheNumberAndTheAge(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		pr   domain.PR
		want []string
	}{
		{"draft", domain.PR{Number: 1, IsDraft: true, UpdatedAt: now.Add(-30 * time.Second)}, []string{"◌", "#1", "now"}},
		{"approved", domain.PR{Number: 2, Review: domain.ReviewApproved, UpdatedAt: now.Add(-5 * time.Minute)}, []string{"✓", "#2", "5m ago"}},
		{"changes requested", domain.PR{Number: 3, Review: domain.ReviewChangesRequested, UpdatedAt: now.Add(-3 * time.Hour)}, []string{"×", "#3", "3h ago"}},
		{"review required", domain.PR{Number: 4, Review: domain.ReviewRequired, UpdatedAt: now.Add(-49 * time.Hour)}, []string{"•", "#4", "2d ago"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := sized(loadedModel(&fakeSource{prs: []domain.PR{c.pr}}), 120)
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
// sideways (spec 6.4).
func TestTheColumnsLineUpDownThePage(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	prs := []domain.PR{
		{Number: 1, Title: "short", UpdatedAt: now.Add(-time.Hour)},
		{Number: 22, Title: "レンダリングのパイプラインをまるごと置き換える", UpdatedAt: now.Add(-time.Hour)},
		{Number: 333, Title: "a middling sort of title", UpdatedAt: now.Add(-time.Hour)},
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

// TestTheSummaryBlockIsAlwaysTheSameHeight keeps the table still: the block is
// drawn under it, and one that grew with the selection would move the key bar.
func TestTheSummaryBlockIsAlwaysTheSameHeight(t *testing.T) {
	f := &fakeSource{
		prs: []domain.PR{
			{Number: 1, Title: "with checks", Head: "a", Base: "main", Additions: 2, Deletions: 1, Checks: domain.Checks{
				Total: 2, Passed: 2, State: domain.CheckSuccess,
				Runs: []domain.CheckRun{{Name: "lint"}, {Name: "test"}},
			}},
			{Number: 2, Title: "without"},
		},
		issues: []domain.Issue{{Number: 9, Title: "an issue", Author: domain.Author{Login: "bob"}}},
	}
	m := sized(loadedModel(f), 120)
	for name, model := range map[string]Model{
		"a PR with checks": m,
		"a PR without":     next(m, "j"),
	} {
		if got := len(model.summary()); got != summaryHeight {
			t.Errorf("%s: the summary is %d lines, want %d", name, got, summaryHeight)
		}
	}
}

// TestTheSummaryNamesTheBranchesAndTheSizeOfTheChange is the line the mockup
// puts under the table.
func TestTheSummaryNamesTheBranchesAndTheSizeOfTheChange(t *testing.T) {
	f := &fakeSource{prs: []domain.PR{{
		Number: 1, Title: "a change", Author: domain.Author{Login: "kukv"},
		Head: "feat/graph", Base: "main", Additions: 218, Deletions: 31,
		Checks: domain.Checks{
			Total: 1, Passed: 1, State: domain.CheckSuccess,
			Runs: []domain.CheckRun{{Name: "lint", State: domain.CheckSuccess}},
		},
	}}}
	got := ansi.Strip(strings.Join(sized(loadedModel(f), 120).summary(), "\n"))
	for _, want := range []string{"@kukv", "feat/graph", "main", "+218", "−31", "lint"} {
		if !strings.Contains(got, want) {
			t.Errorf("the summary is missing %q:\n%s", want, got)
		}
	}
}

// TestTheListFitsTheTerminal is the same rule the board follows: everything
// drawn has to be on screen, so a repository with a hundred pull requests
// scrolls rather than pushing the key bar off the bottom.
func TestTheListFitsTheTerminal(t *testing.T) {
	var prs []domain.PR
	for i := range 60 {
		prs = append(prs, domain.PR{Number: i + 1, Title: "a pull request"})
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
		prs := make([]domain.PR, count)
		for i := range prs {
			prs[i] = domain.PR{Number: i + 1, Title: "a pull request"}
		}
		m := currentModel(&fakeSource{prs: prs}, 120)

		if got := len(strings.Split(m.View(), "\n")); got != 40 {
			t.Errorf("%d pull requests: the view is %d rows, want 40", count, got)
		}
	}
}

// TestTheSummaryBlockSitsAboveTheKeyBar is the other half of that: the block
// is anchored to the bottom the way the board's drawer is, so neither it nor
// the table above it moves as rows arrive.
func TestTheSummaryBlockSitsAboveTheKeyBar(t *testing.T) {
	const height = 40
	want := height - footerHeight - summaryHeight

	for _, count := range []int{2, 20} {
		prs := make([]domain.PR, count)
		for i := range prs {
			prs[i] = domain.PR{Number: i + 1, Title: "a pull request", Head: "topic", Base: "main"}
		}
		m := currentModel(&fakeSource{prs: prs}, 120)

		// The view has the sidebar and its rule in front of every line of the
		// right pane, so the summary's first line is matched by its tail.
		first := strings.TrimRight(ansi.Strip(m.summary()[0]), " ")
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		got := -1
		for i, line := range lines {
			if strings.HasSuffix(strings.TrimRight(line, " "), first) {
				got = i
				break
			}
		}
		if got != want {
			t.Errorf("%d pull requests: the summary starts on row %d, want %d:\n%s",
				count, got, want, strings.Join(lines, "\n"))
		}
	}
}
