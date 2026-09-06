package repo

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// fakeSource implements Source and records calls.
type fakeSource struct {
	prs      []gh.PR
	issues   []gh.Issue
	err      error
	webCalls []string // "pr:<repo>:<n>" / "issue:<repo>:<n>"
}

func (f *fakeSource) ListPRs(ctx context.Context) ([]gh.PR, error) { return f.prs, f.err }

func (f *fakeSource) ListIssues(ctx context.Context) ([]gh.Issue, error) {
	return f.issues, f.err
}

func (f *fakeSource) RepoName(ctx context.Context) (string, error) { return "kukv/demo", f.err }

func (f *fakeSource) OpenPRWeb(repo string, n int) error {
	f.webCalls = append(f.webCalls, "pr:"+repo+":"+itoa(n))
	return nil
}

func (f *fakeSource) OpenIssueWeb(repo string, n int) error {
	f.webCalls = append(f.webCalls, "issue:"+repo+":"+itoa(n))
	return nil
}

func itoa(n int) string { return string(rune('0' + n)) } // tests only use n < 10

func samplePRs() []gh.PR {
	return []gh.PR{
		{
			Number: 1, Title: "first pr", Author: gh.Author{Login: "kukv"},
			UpdatedAt: time.Now(), Review: gh.ReviewApproved,
		},
		{
			Number: 2, Title: "second pr", Author: gh.Author{Login: "bob"},
			UpdatedAt: time.Now(),
		},
	}
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

// loadedModel returns a Model with the PR list already loaded, sized as the
// app sizes it. The list lays itself out to the terminal it was given, so an
// unsized one draws nothing at all.
func loadedModel(f *fakeSource) Model {
	m := sized(New(f), 120)
	m, _ = m.Update(prListMsg(f.prs))
	return m
}

func TestPRListRenders(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	view := m.View()
	for _, want := range []string{"first pr", "second pr", "@kukv", "#1"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}

func TestRepoNameShownInHeader(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sized(New(f), 120)
	m, _ = m.Update(repoNameMsg("kukv/demo"))
	if !strings.Contains(m.View(), "kukv/demo") {
		t.Errorf("header missing the repository name:\n%s", m.View())
	}
}

func TestRepoNameFailureIsIgnored(t *testing.T) {
	f := &fakeSource{err: errors.New("gh repo: no git remotes found")}
	msg := fetchRepoName(f)()
	if got, ok := msg.(repoNameMsg); !ok || got != "" {
		t.Errorf("msg = %#v, want an empty repoNameMsg (the header name is not worth an error screen)", msg)
	}
}

func TestEmptyPRList(t *testing.T) {
	f := &fakeSource{}
	m := loadedModel(f)
	if !strings.Contains(m.View(), "No open pull requests") {
		t.Errorf("view missing empty state:\n%s", m.View())
	}
}

// spinnerFrame is the first frame of spinner.Dot, which is what a freshly
// built model draws.
const spinnerFrame = "⣾"

func TestLoadingShowsSpinnerAndText(t *testing.T) {
	m := sized(New(&fakeSource{prs: samplePRs()}), 120)
	view := m.View()
	if !strings.Contains(view, "loading...") {
		t.Errorf("view missing the loading text before the list arrives:\n%s", view)
	}
	if !strings.Contains(view, spinnerFrame) {
		t.Errorf("view missing the spinner frame while loading:\n%s", view)
	}
}

// TestInitStartsTheSpinnerAndTheFetches covers what Init batches: the spinner
// tick, the repository name and the first list.
func TestInitStartsTheSpinnerAndTheFetches(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := New(f)
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("Init = %T, want a batch", m.Init()())
	}
	if len(batch) != 3 {
		t.Fatalf("Init batched %d commands, want the tick, the name and the list", len(batch))
	}
	if _, ok := batch[1]().(repoNameMsg); !ok {
		t.Errorf("batch[1] = %T, want repoNameMsg", batch[1]())
	}
	if _, ok := batch[2]().(prListMsg); !ok {
		t.Errorf("batch[2] = %T, want prListMsg", batch[2]())
	}
}

func TestSpinnerTickAdvancesTheFrame(t *testing.T) {
	m := New(&fakeSource{})
	before := m.spin.View()
	m, cmd := m.Update(m.spin.Tick())
	if cmd == nil {
		t.Fatal("a tick produced no follow-up command; the animation would stop")
	}
	if m.spin.View() == before {
		t.Errorf("the spinner frame did not advance: still %q", before)
	}
}

func TestCursorMovesAndClamps(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	for _, k := range []string{"j", "j", "j"} { // only two items: it stops at the end
		m, _ = m.Update(key(k))
	}
	if m.cursors[tabPRs] != 1 {
		t.Errorf("cursor = %d, want 1", m.cursors[tabPRs])
	}
	m, _ = m.Update(key("k"))
	if m.cursors[tabPRs] != 0 {
		t.Errorf("cursor = %d, want 0", m.cursors[tabPRs])
	}
}

func TestTabSwitchLoadsIssues(t *testing.T) {
	f := &fakeSource{issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
	m := loadedModel(f)
	m, cmd := m.Update(key("tab"))
	if m.tab != tabIssues || cmd == nil {
		t.Fatalf("tab = %v, cmd = %v; want tabIssues with fetch cmd", m.tab, cmd)
	}
	m, _ = m.Update(cmd()) // run the fetch synchronously and feed the result back
	if !strings.Contains(m.View(), "an issue") {
		t.Errorf("view missing issue:\n%s", m.View())
	}
}

func TestFetchFailureBecomesErrorMsg(t *testing.T) {
	f := &fakeSource{err: errors.New("gh pr: no git remotes found")}
	m := New(f)
	_, cmd := m.Update(fetchList(f, tabPRs)())
	if cmd == nil {
		t.Fatal("cmd = nil after a failed fetch, want ErrorMsg cmd")
	}
	msg, ok := cmd().(ErrorMsg)
	if !ok {
		t.Fatalf("msg = %T, want ErrorMsg", cmd())
	}
	if !strings.Contains(msg.Err.Error(), "no git remotes found") {
		t.Errorf("Err = %v, want the source's error", msg.Err)
	}
}

func TestEnterAsksTheParentForTheDetail(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	m, _ = m.Update(key("j")) // second PR
	_, cmd := m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("cmd = nil, want OpenDetailMsg cmd")
	}
	msg, ok := cmd().(OpenDetailMsg)
	if !ok {
		t.Fatalf("msg = %T, want OpenDetailMsg", cmd())
	}
	if msg.Ref != (gh.ItemRef{Kind: gh.ItemPR, Number: 2}) {
		t.Errorf("Ref = %+v, want the PR under the cursor", msg.Ref)
	}
}

func TestEnterOnAnIssueCarriesTheIssueKind(t *testing.T) {
	f := &fakeSource{issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
	m := loadedModel(f)
	m, cmd := m.Update(key("tab"))
	m, _ = m.Update(cmd())
	_, cmd = m.Update(key("enter"))
	if cmd == nil {
		t.Fatal("cmd = nil, want OpenDetailMsg cmd")
	}
	msg, ok := cmd().(OpenDetailMsg)
	if !ok {
		t.Fatalf("msg = %T, want OpenDetailMsg", cmd())
	}
	if msg.Ref != (gh.ItemRef{Kind: gh.ItemIssue, Number: 3}) {
		t.Errorf("Ref = %+v, want the issue under the cursor", msg.Ref)
	}
}

func TestEnterOnEmptyListDoesNothing(t *testing.T) {
	m := loadedModel(&fakeSource{})
	_, cmd := m.Update(key("enter"))
	if cmd != nil {
		t.Errorf("cmd = non-nil on an empty list, want nil")
	}
}

func TestOOpensBrowserForSelection(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	_, cmd := m.Update(key("o"))
	if cmd == nil {
		t.Fatal("cmd = nil, want openWeb cmd")
	}
	cmd()
	if len(f.webCalls) != 1 || f.webCalls[0] != "pr::1" {
		t.Errorf("webCalls = %v, want [pr::1]", f.webCalls)
	}
}

func TestRefreshRefetchesTheCurrentTab(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	m, cmd := m.Update(key("r"))
	if !m.loading[tabPRs] || cmd == nil {
		t.Fatalf("loading = %v, cmd = %v; want loading with fetch cmd", m.loading[tabPRs], cmd)
	}
	if _, ok := cmd().(prListMsg); !ok {
		t.Errorf("msg = %T, want prListMsg", cmd())
	}
}

// TestRefreshThenTabSwitchClearsCorrectLoading reproduces the stuck-spinner
// bug: pressing r on the PRs tab, then tab to the already-loaded Issues tab
// before the PR fetch returns, must not leave Issues stuck on "loading..."
// when the late prListMsg finally arrives.
func TestRefreshThenTabSwitchClearsCorrectLoading(t *testing.T) {
	f := &fakeSource{prs: samplePRs(), issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
	m := loadedModel(f)
	m, _ = m.Update(issueListMsg(f.issues)) // Issues tab already loaded once before

	m, refreshCmd := m.Update(key("r")) // refresh PRs; fetch is still "in flight"
	if refreshCmd == nil {
		t.Fatal("cmd = nil, want fetch cmd for r")
	}

	m, tabCmd := m.Update(key("tab")) // switch to Issues before the refresh returns
	if m.tab != tabIssues {
		t.Fatalf("tab = %v, want tabIssues", m.tab)
	}
	if tabCmd != nil {
		t.Fatalf("switching to an already-loaded tab issued cmd = %v, want nil", tabCmd)
	}
	if view := m.View(); strings.Contains(view, "loading...") || !strings.Contains(view, "an issue") {
		t.Errorf("Issues view should render items immediately, got:\n%s", view)
	}

	m, _ = m.Update(refreshCmd()) // late prListMsg arrives while Issues is visible
	if view := m.View(); strings.Contains(view, "loading...") || !strings.Contains(view, "an issue") {
		t.Errorf("Issues view got stuck on the loading text after a late prListMsg, got:\n%s", view)
	}

	m, _ = m.Update(key("tab")) // switch back to PRs
	if view := m.View(); strings.Contains(view, "loading...") || !strings.Contains(view, "first pr") {
		t.Errorf("PRs view stuck loading or missing refreshed items, got:\n%s", view)
	}
}

func TestCursorClampsWhenTheListShrinks(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	m, _ = m.Update(key("j")) // cursor on the second PR
	m, _ = m.Update(prListMsg(samplePRs()[:1]))
	if m.cursors[tabPRs] != 0 {
		t.Errorf("cursor = %d after the list shrank, want 0", m.cursors[tabPRs])
	}
}

// overlongPRs is samplePRs plus an item whose title and author are wider than
// any terminal the width test uses, in both scripts. Without it the fixture's
// longest line is well inside 50 columns and the test would pass even with the
// truncation removed.
func overlongPRs() []gh.PR {
	return append(samplePRs(), gh.PR{
		Number: 9,
		Title: "レンダリングのパイプラインをまるごと置き換える " +
			"refactor that nobody asked for",
		Author:    gh.Author{Login: "a-contributor-with-a-very-long-handle"},
		UpdatedAt: time.Now(),
	})
}

func overlongIssues() []gh.Issue {
	return []gh.Issue{{
		Number: 9,
		Title: "ラベルの一覧が横に伸びつづける問題 " +
			"and an English clause long enough to run off the screen",
		Author:    gh.Author{Login: "another-contributor-with-a-long-handle"},
		UpdatedAt: time.Now(),
	}}
}

// TestNoLineExceedsTheTerminalWidth guards spec §6.4 across both scripts: a
// Japanese character occupies two columns, so a line that fits in English can
// still run off the screen in Japanese.
func TestNoLineExceedsTheTerminalWidth(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{50, 80, 100, 120} {
			f := &fakeSource{prs: overlongPRs(), issues: overlongIssues()}
			prs := sized(loadedModel(f), width)
			issues, cmd := prs.Update(key("tab"))
			issues, _ = issues.Update(cmd())

			for name, view := range map[string]string{
				"prs":     prs.View(),
				"issues":  issues.View(),
				"loading": sized(New(f), width).View(),
				"empty":   sized(loadedModel(&fakeSource{}), width).View(),
			} {
				for _, line := range strings.Split(view, "\n") {
					if w := ansi.StringWidth(line); w > width {
						t.Errorf("%s lang %s width %d: line is %d columns: %q",
							name, lang, width, w, line)
					}
				}
			}
		}
	}
}

func sized(m Model, width int) Model {
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	return m
}

// TestNoUnresolvedIDsInRenderedViews guards spec §6.5. It renders each of the
// list's screens in both languages and fails when a message ID the code asked
// for is missing from that language's catalog. Walking i18n.IDs() cannot catch
// this: it only proves the catalog can resolve its own IDs, never that the IDs
// the code spells match them.
func TestNoUnresolvedIDsInRenderedViews(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for name, view := range renderEveryScreen() {
			t.Run(lang.String()+"/"+name, func(t *testing.T) {
				i18n.AssertNoUnresolvedIDs(t, view)
			})
		}
	}
}

func renderEveryScreen() map[string]string {
	f := &fakeSource{prs: samplePRs(), issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
	list := loadedModel(f)
	issues, cmd := list.Update(key("tab"))
	issues, _ = issues.Update(cmd())
	empty := loadedModel(&fakeSource{})

	return map[string]string{
		"list_prs":    list.View(),
		"list_issues": issues.View(),
		"empty":       empty.View(),
		"loading":     New(f).View(),
	}
}
