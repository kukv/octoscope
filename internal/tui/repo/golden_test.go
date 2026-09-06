package repo

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/golden"
	"github.com/kukv/octoscope/internal/i18n"
)

var goldenWidths = []int{160, 120, 80}

var goldenLanguages = []struct {
	name string
	tag  language.Tag
}{
	{"en", language.English},
	{"ja", language.Japanese},
}

// goldenUpdatedAt and goldenFetchedAt are constants because the rows carry
// relative times: a recording made against the wall clock would go stale a
// minute after it was made.
var (
	goldenUpdatedAt = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	goldenFetchedAt = time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)
)

func goldenPRs() []gh.PR {
	return []gh.PR{
		{
			Number: 1, Title: "first pr", Author: gh.Author{Login: "kukv"},
			UpdatedAt: goldenUpdatedAt, Review: gh.ReviewApproved,
		},
		{
			Number: 2, Title: "second pr", Author: gh.Author{Login: "bob"},
			UpdatedAt: goldenUpdatedAt, IsDraft: true,
			Labels: []gh.Label{{Name: "bug", Color: "d73a4a"}},
			Head:   "fix/thing", Base: "main", Additions: 12, Deletions: 3,
			Checks: gh.Checks{
				Total: 2, Passed: 1, Failed: 1, State: gh.CheckFailure,
				Runs: []gh.CheckRun{
					{Name: "lint", State: gh.CheckSuccess},
					{Name: "test", State: gh.CheckFailure},
				},
			},
		},
		{
			Number: 9,
			Title: "レンダリングのパイプラインをまるごと置き換える " +
				"refactor that nobody asked for",
			Author:    gh.Author{Login: "a-contributor-with-a-very-long-handle"},
			UpdatedAt: goldenUpdatedAt,
		},
	}
}

func goldenIssues() []gh.Issue {
	return []gh.Issue{{
		Number: 7,
		Title: "ラベルの一覧が横に伸びつづける問題 " +
			"and an English clause long enough to run off the screen",
		Author:    gh.Author{Login: "another-contributor-with-a-long-handle"},
		UpdatedAt: goldenUpdatedAt,
	}}
}

func goldenModel(width int) Model {
	m := loadedModel(&fakeSource{prs: goldenPRs(), issues: goldenIssues()})
	m, _ = m.Update(repoNameMsg("kukv/octoscope"))
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m.fetchedAt = [2]time.Time{goldenFetchedAt, goldenFetchedAt}
	return m
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })

				prs := goldenModel(w)
				issues, cmd := prs.Update(key("tab"))
				issues, _ = issues.Update(cmd())
				issues.fetchedAt = [2]time.Time{goldenFetchedAt, goldenFetchedAt}

				golden.Assert(t, fmt.Sprintf("repo_prs_%s_%d", lang.name, w), prs.View())
				golden.Assert(t, fmt.Sprintf("repo_issues_%s_%d", lang.name, w), issues.View())
			})
		}
	}
}
