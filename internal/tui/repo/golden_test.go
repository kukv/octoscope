package repo

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/browser"
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
	f := &fakeSource{prs: goldenPRs(), issues: goldenIssues()}
	m := sized(New(f, Options{
		Repositories: []string{"kukv/octoscope", "kukv/koto"},
		Current:      "kukv/octoscope",
	}), width)
	m, _ = m.Update(prListMsg{prs: f.prs})
	m, _ = m.Update(repoCountsMsg([]gh.RepoCount{
		{Repo: "kukv/octoscope", PRs: 12, Issues: 3},
		{Repo: "kukv/koto", Unavailable: true},
	}))
	m.fetchedAt = [2]time.Time{goldenFetchedAt, goldenFetchedAt}
	return m
}

// goldenFailure is what GitHub says when its front end will not answer. It is
// long on purpose: the notice has to survive a narrow terminal.
const goldenFailure = "gh api: HTTP 502: Bad gateway (https://api.github.com/graphql)"

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })

				// The list the user is left with when a refetch fails: the
				// rows it already had, and a line saying what GitHub said.
				failed := goldenModel(w)
				failed, _ = failed.Update(errMsg{gen: failed.gen, err: errors.New(goldenFailure)})
				golden.Assert(t, fmt.Sprintf("repo_failed_%s_%d", lang.name, w), failed.View())

				// The other failure that reaches the same line, which must
				// not blame the fetch for what the browser did.
				noBrowser := goldenModel(w)
				noBrowser, _ = noBrowser.Update(errMsg{
					gen:  noBrowser.gen,
					kind: noticeOpen,
					err:  &browser.NoneError{URL: "https://github.com/kukv/octoscope/pull/1"},
				})
				golden.Assert(t, fmt.Sprintf("repo_no_browser_%s_%d", lang.name, w), noBrowser.View())

				prs := goldenModel(w)
				issues, cmd := prs.Update(key("tab"))
				issues, _ = issues.Update(cmd())
				issues.fetchedAt = [2]time.Time{goldenFetchedAt, goldenFetchedAt}

				golden.Assert(t, fmt.Sprintf("repo_prs_%s_%d", lang.name, w), prs.View())
				golden.Assert(t, fmt.Sprintf("repo_issues_%s_%d", lang.name, w), issues.View())

				empty := sized(New(&fakeSource{}, Options{}), w)
				golden.Assert(t, fmt.Sprintf("repo_empty_%s_%d", lang.name, w), empty.View())
			})
		}
	}
}
