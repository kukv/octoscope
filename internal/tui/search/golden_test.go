package search

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

// goldenUpdatedAt and goldenFetchedAt are constants, three hours apart, so a
// recording shows a relative age ("3 hours ago") without going stale a
// minute after it was made.
var (
	goldenUpdatedAt = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	goldenFetchedAt = goldenUpdatedAt.Add(3 * time.Hour)
)

// goldenItems mirrors repo's golden fixtures: a title long enough to clip at
// eighty columns, and a repository name that exercises the column's width.
func goldenItems() []gh.WorkItem {
	return []gh.WorkItem{
		{
			Ref:       gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 42},
			Title:     "レンダリングのパイプラインをまるごと置き換える refactor that nobody asked for",
			Review:    gh.ReviewApproved,
			UpdatedAt: goldenUpdatedAt,
		},
		{
			Ref:       gh.ItemRef{Kind: gh.ItemIssue, Repo: "kukv/a-repository-with-a-name-nobody-would-type-twice", Number: 7},
			Title:     "ラベルの一覧が横に伸びつづける問題",
			UpdatedAt: goldenUpdatedAt,
		},
	}
}

// goldenModel reaches the loaded state through Init and its own reply, not
// by assigning fields the search never sets that way itself.
func goldenModel(t *testing.T, width int, items []gh.WorkItem) Model {
	t.Helper()
	m := New(&fakeSource{items: items})
	m = resolve(t, m, m.Init())
	m, _ = m.Update(windowSize(width))
	m.fetchedAt = goldenFetchedAt
	return m
}

// goldenLabels is what the candidates golden state's repository offers.
func goldenLabels() []gh.Label {
	return []gh.Label{
		{Name: "bug", Color: "d73a4a"},
		{Name: "enhancement", Color: "a2eeef"},
		{Name: "documentation", Color: "0075ca"},
	}
}

func windowSize(width int) tea.WindowSizeMsg {
	return tea.WindowSizeMsg{Width: width, Height: 40}
}

// typeInto presses each rune of s in turn, the way the raw query editor is
// actually filled in.
func typeInto(m Model, s string) Model {
	for _, r := range s {
		m, _ = press(m, string(r))
	}
	return m
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })

				loaded := goldenModel(t, w, goldenItems())
				golden.Assert(t, fmt.Sprintf("search_%s_%d", lang.name, w), loaded.View())

				editing := loaded
				editing, _ = press(editing, "e")
				editing = typeInto(editing, " author:kukv")
				golden.Assert(t, fmt.Sprintf("search_editing_%s_%d", lang.name, w), editing.View())

				empty := goldenModel(t, w, nil)
				golden.Assert(t, fmt.Sprintf("search_empty_%s_%d", lang.name, w), empty.View())

				// The cursor is moved to FilterLabel and the fetch resolved
				// before the window is sized, since a narrow width would
				// otherwise send j/k to the result pane instead of the
				// filter pane (the same fold-away this tab always has).
				candidates := New(&fakeSource{items: goldenItems(), labels: goldenLabels()})
				candidates = resolve(t, candidates, candidates.Init())
				candidates = withRepo(t, candidates)
				candidates = onFilter(t, candidates, FilterLabel)
				candidates = resolveCandidates(t, candidates)
				candidates, _ = candidates.Update(windowSize(w))
				candidates.fetchedAt = goldenFetchedAt
				golden.Assert(t, fmt.Sprintf("search_candidates_%s_%d", lang.name, w), candidates.View())

				picker := saveQuery(t, loaded, "urgent", "is:open label:urgent")
				picker = saveQuery(t, picker, "自分のPR", "author:kukv")
				picker, _ = press(picker, "ctrl+o")
				golden.Assert(t, fmt.Sprintf("search_picker_%s_%d", lang.name, w), picker.View())

				naming := loaded
				naming, _ = press(naming, "s")
				naming = typeInto(naming, "自分")
				golden.Assert(t, fmt.Sprintf("search_naming_%s_%d", lang.name, w), naming.View())
			})
		}
	}
}

// saveQuery goes through the same keys a user does to save one: e to open
// the raw editor (which starts filled with the current query, so it is
// cleared first rather than typed into on top of), the query, s to name it,
// enter to commit each. It leaves the model in browse mode with the query
// saved, ready for the next one or for ctrl+o to open the picker.
func saveQuery(t *testing.T, m Model, name, query string) Model {
	t.Helper()
	m, cmd := press(m, "e")
	for range []rune(m.input.Value()) {
		m, _ = press(m, "backspace")
	}
	m = typeInto(m, query)
	m, cmd = press(m, "enter")
	m = resolve(t, m, cmd)
	m, _ = press(m, "s")
	m = typeInto(m, name)
	m, cmd = press(m, "enter")
	m = resolve(t, m, cmd)
	return m
}
