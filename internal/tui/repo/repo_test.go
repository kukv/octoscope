package repo

import (
	"context"
	"errors"
	"fmt"
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
	webCalls []string // the URLs handed to the browser

	counts     []gh.RepoCount
	countErr   error
	countCalls [][]string

	prRepos    []string // the repositories ListPRs was asked for, in call order
	issueRepos []string
}

func (f *fakeSource) ListPRs(ctx context.Context, repo string) ([]gh.PR, error) {
	f.prRepos = append(f.prRepos, repo)
	return f.prs, f.err
}

func (f *fakeSource) ListIssues(ctx context.Context, repo string) ([]gh.Issue, error) {
	f.issueRepos = append(f.issueRepos, repo)
	return f.issues, f.err
}

func (f *fakeSource) OpenWeb(url string) error {
	f.webCalls = append(f.webCalls, url)
	return nil
}

func (f *fakeSource) RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error) {
	f.countCalls = append(f.countCalls, repos)
	return f.counts, f.countErr
}

func samplePRs() []gh.PR {
	return []gh.PR{
		{
			Number: 1, Title: "first pr", Author: gh.Author{Login: "kukv"},
			UpdatedAt: time.Now(), Review: gh.ReviewApproved,
			URL: "https://github.com/kukv/demo/pull/1",
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
	m := sized(New(f, Options{}), 120)
	m, _ = m.Update(prListMsg{prs: f.prs})
	return m
}

// currentModel is loadedModel with a current repository, for tests that
// switch tabs or refresh: with no rows, Refresh and showTab have nothing to
// fetch (TestRefreshWithNoRowsFetchesNothing, TestSwitchingTabWithNoRowsFetchesNothing).
func currentModel(f *fakeSource, width int) Model {
	m := sized(New(f, Options{Current: "kukv/octoscope"}), width)
	m, _ = m.Update(prListMsg{repo: m.selectedRepo(), prs: f.prs})
	return m
}

// sidebarModel returns a Model with a sidebar and its PR list already
// loaded. The response carries the selected row's own name: once selecting a
// row throws away a response for a different repository, a response with no
// repo at all would be discarded and every test built on this would find an
// empty table.
func sidebarModel(f *fakeSource, width int) Model {
	m := sized(New(f, Options{
		Repositories: []string{"kukv/octoscope", "kukv/koto"},
		Current:      "kukv/octoscope",
	}), width)
	m, _ = m.Update(prListMsg{repo: "kukv/octoscope", prs: f.prs})
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

// The header names the repository under the sidebar's cursor, which is
// Current once SetCurrent has placed it there.
func TestRepoNameShownInHeader(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sized(New(f, Options{Current: "kukv/demo"}), 120)
	if !strings.Contains(m.View(), "kukv/demo") {
		t.Errorf("header missing the repository name:\n%s", m.View())
	}
}

func TestEmptyPRList(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Current: "kukv/octoscope"}), 120)
	m, _ = m.Update(prListMsg{repo: "kukv/octoscope", prs: f.prs})
	if !strings.Contains(m.View(), "No open pull requests") {
		t.Errorf("view missing empty state:\n%s", m.View())
	}
}

// An empty settings file and no current repository is a different state from
// a repository with no open pull requests: there is nothing to list at all.
func TestEmptyListSaysSo(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	view := m.View()
	if !strings.Contains(view, i18n.T("repos.none")) {
		t.Errorf("an empty Repos tab says nothing:\n%s", view)
	}
	if strings.Contains(view, i18n.T("common.loading")) {
		t.Errorf("an empty Repos tab spins forever:\n%s", view)
	}
}

// spinnerFrame is the first frame of spinner.Dot, which is what a freshly
// built model draws.
const spinnerFrame = "⣾"

func TestLoadingShowsSpinnerAndText(t *testing.T) {
	m := sized(New(&fakeSource{prs: samplePRs()}, Options{Current: "kukv/octoscope"}), 120)
	view := m.View()
	if !strings.Contains(view, "loading...") {
		t.Errorf("view missing the loading text before the list arrives:\n%s", view)
	}
	if !strings.Contains(view, spinnerFrame) {
		t.Errorf("view missing the spinner frame while loading:\n%s", view)
	}
}

// TestInitStartsTheSpinnerAndTheFetches covers what Init batches: the spinner
// tick and the first list. See TestInitFetchesTheSidebarsCounts for the
// counts fetch.
func TestInitStartsTheSpinnerAndTheFetches(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := New(f, Options{Current: "kukv/octoscope"})
	msgs := drain(t, m.Init())
	var haveList bool
	for _, msg := range msgs {
		if _, ok := msg.(prListMsg); ok {
			haveList = true
		}
	}
	if !haveList {
		t.Errorf("Init's batch is missing prListMsg: %v", msgs)
	}
}

func TestInitFetchesTheSidebarsCounts(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := New(f, Options{
		Repositories: []string{"kukv/octoscope", "kukv/koto"},
		Current:      "kukv/octoscope",
	})
	drain(t, m.Init())
	if len(f.countCalls) != 1 {
		t.Errorf("RepoCounts called %d times on Init, want 1", len(f.countCalls))
	}
	if got := f.countCalls[0]; len(got) != 2 || got[0] != "kukv/octoscope" {
		t.Errorf("RepoCounts got %v, want every row's name", got)
	}
}

func TestSpinnerTickAdvancesTheFrame(t *testing.T) {
	m := New(&fakeSource{}, Options{})
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
	m := currentModel(f, 120)
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
	m := New(f, Options{})
	_, cmd := m.Update(fetchList(f, tabPRs, "")())
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
	m := currentModel(f, 120)
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
	if msg.Ref != (gh.ItemRef{Kind: gh.ItemIssue, Repo: "kukv/octoscope", Number: 3}) {
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

func TestDAsksForTheDiff(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	m, _ = m.Update(key("j")) // second PR
	_, cmd := m.Update(key("d"))
	if cmd == nil {
		t.Fatal("d produced no command")
	}
	msg, ok := cmd().(OpenDiffMsg)
	if !ok {
		t.Fatalf("got %T, want OpenDiffMsg", cmd())
	}
	want, _ := m.SelectedRef()
	if msg.Ref != want {
		t.Errorf("d asked for %+v, want the selected row %+v", msg.Ref, want)
	}
}

// TestDDoesNothingOnAnIssue is what stops the diff view opening on something
// that has no diff.
func TestDDoesNothingOnAnIssue(t *testing.T) {
	f := &fakeSource{issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
	m := currentModel(f, 120)
	m, cmd := m.Update(key("tab"))
	m, _ = m.Update(cmd())
	if _, cmd := m.Update(key("d")); cmd != nil {
		t.Errorf("d on an issue produced %T", cmd())
	}
}

func TestSAsksForTheChecks(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	m, _ = m.Update(key("j")) // second PR
	_, cmd := m.Update(key("s"))
	if cmd == nil {
		t.Fatal("s produced no command")
	}
	msg, ok := cmd().(OpenChecksMsg)
	if !ok {
		t.Fatalf("got %T, want OpenChecksMsg", cmd())
	}
	want, _ := m.SelectedRef()
	if msg.Ref != want {
		t.Errorf("s asked for %+v, want the selected row %+v", msg.Ref, want)
	}
}

// TestSDoesNothingOnAnIssue is what stops the checks view opening on
// something that has no checks.
func TestSDoesNothingOnAnIssue(t *testing.T) {
	f := &fakeSource{issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
	m := currentModel(f, 120)
	m, cmd := m.Update(key("tab"))
	m, _ = m.Update(cmd())
	if _, cmd := m.Update(key("s")); cmd != nil {
		t.Errorf("s on an issue produced %T", cmd())
	}
}

// The ref leaves this view for the detail, diff and checks views, and those
// draw the repository in their titles. Leaving it empty put a bare " #1" at
// the top of the checks view. Both tabs hand out refs, so both are checked.
func TestTheSelectedRefCarriesTheRepositoryName(t *testing.T) {
	tests := []struct {
		name     string
		toIssues bool
		wantKind gh.ItemKind
	}{
		{name: "the PRs tab", wantKind: gh.ItemPR},
		{name: "the Issues tab", toIssues: true, wantKind: gh.ItemIssue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSource{prs: samplePRs(), issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
			m := sized(New(f, Options{Current: "kukv/demo"}), 120)
			m, _ = m.Update(prListMsg{repo: "kukv/demo", prs: f.prs})
			if tt.toIssues {
				var cmd tea.Cmd
				m, cmd = m.Update(key("tab"))
				m, _ = m.Update(cmd())
			}

			ref, ok := m.SelectedRef()
			if !ok {
				t.Fatalf("%s: SelectedRef reported nothing selected", tt.name)
			}
			if ref.Kind != tt.wantKind {
				t.Fatalf("%s: Kind = %v, want %v", tt.name, ref.Kind, tt.wantKind)
			}
			if ref.Repo != "kukv/demo" {
				t.Errorf("%s: Repo = %q, want kukv/demo", tt.name, ref.Repo)
			}
		})
	}
}

// TestKeyBarNamesTheChecksKey pins s alongside d in the list's key bar: a
// key with no hint in the footer is a key nobody can find.
func TestKeyBarNamesTheChecksKey(t *testing.T) {
	m := loadedModel(&fakeSource{prs: samplePRs()})
	if got := m.View(); !strings.Contains(got, "s:checks") {
		t.Errorf("key bar = %q, want it to mention s:checks", got)
	}
}

// TestOOpensTheSelectionsOwnURL pins that o opens the address GitHub gave
// the selected item, rather than one octoscope spelled out itself.
func TestOOpensTheSelectionsOwnURL(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := loadedModel(f)
	_, cmd := m.Update(key("o"))
	if cmd == nil {
		t.Fatal("cmd = nil, want openWeb cmd")
	}
	cmd()
	want := f.prs[0].URL
	if want == "" {
		t.Fatal("the sample pull request has no URL, so this proves nothing")
	}
	if len(f.webCalls) != 1 || f.webCalls[0] != want {
		t.Errorf("webCalls = %v, want [%s]", f.webCalls, want)
	}
}

func TestRefreshRefetchesTheCurrentTab(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := currentModel(f, 120)
	m, cmd := m.Update(key("r"))
	if !m.loading[tabPRs] || cmd == nil {
		t.Fatalf("loading = %v, cmd = %v; want loading with fetch cmd", m.loading[tabPRs], cmd)
	}
	msgs := drain(t, cmd)
	found := false
	for _, msg := range msgs {
		if _, ok := msg.(prListMsg); ok {
			found = true
		}
	}
	if !found {
		t.Errorf("msgs = %v, want a prListMsg among them", msgs)
	}
}

// With no rows there is nothing to fetch, and fetchList("") would fail:
// gh pr list with no --repo reads the working directory, which the app.fail
// screen would then swallow the whole UI for.
func TestRefreshWithNoRowsFetchesNothing(t *testing.T) {
	m := sized(New(&fakeSource{}, Options{}), 120)
	m, cmd := m.Refresh()
	if cmd != nil {
		t.Errorf("cmd = %v, want nil with no rows to refresh", cmd)
	}
	if m.loading[m.tab] {
		t.Error("loading was set with nothing to load")
	}
}

// TestRefreshThenTabSwitchClearsCorrectLoading reproduces the stuck-spinner
// bug: pressing r on the PRs tab, then tab to the already-loaded Issues tab
// before the PR fetch returns, must not leave Issues stuck on "loading..."
// when the late prListMsg finally arrives.
func TestRefreshThenTabSwitchClearsCorrectLoading(t *testing.T) {
	f := &fakeSource{prs: samplePRs(), issues: []gh.Issue{{Number: 3, Title: "an issue"}}}
	m := currentModel(f, 120)
	m, _ = m.Update(issueListMsg{repo: m.selectedRepo(), issues: f.issues}) // Issues tab already loaded once before

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

	for _, msg := range drain(t, refreshCmd) { // late messages arrive while Issues is visible
		m, _ = m.Update(msg)
	}
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
	m, _ = m.Update(prListMsg{prs: samplePRs()[:1]})
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
			prs := currentModel(f, width)
			issues, cmd := prs.Update(key("tab"))
			issues, _ = issues.Update(cmd())

			for name, view := range map[string]string{
				"prs":     prs.View(),
				"issues":  issues.View(),
				"loading": sized(New(f, Options{}), width).View(),
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

// drain runs cmd and, if it produced a batch, runs every command in the
// batch too, flattening the result into the messages they returned.
func drain(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var msgs []tea.Msg
	for _, c := range batch {
		msgs = append(msgs, drain(t, c)...)
	}
	return msgs
}

func TestBadgesShowTheCounts(t *testing.T) {
	f := &fakeSource{prs: samplePRs(), counts: []gh.RepoCount{
		{Repo: "kukv/octoscope", PRs: 12, Issues: 3},
		{Repo: "kukv/koto", Unavailable: true},
	}}
	m := sidebarModel(f, 120)
	m, _ = m.Update(repoCountsMsg(f.counts))
	view := m.View()
	if !strings.Contains(view, "12/3") {
		t.Errorf("the badge is missing:\n%s", view)
	}
	if !strings.Contains(view, "—") {
		t.Errorf("a repository that could not be counted lost its row or got a zero:\n%s", view)
	}
}

// RepoCounts answers positionally and rewrites each name to the spelling
// GitHub resolved. Matching by name would drop the answer for a row the user
// spelled differently.
func TestCountsMatchByPositionAndTakeTheResolvedName(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sized(New(f, Options{Repositories: []string{"KUKV/Octoscope"}}), 120)
	m, _ = m.Update(repoCountsMsg([]gh.RepoCount{{Repo: "kukv/octoscope", PRs: 1, Issues: 2}}))
	if m.rows[0].name != "kukv/octoscope" {
		t.Errorf("row name = %q, want the spelling GitHub resolved", m.rows[0].name)
	}
}

// A shorter or longer answer than there are rows must not panic.
func TestCountsOfADifferentLengthAreIgnored(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	before := m.rows
	m, _ = m.Update(repoCountsMsg([]gh.RepoCount{{Repo: "kukv/octoscope"}}))
	if m.rows[0].counted != before[0].counted {
		t.Error("a mismatched answer was taken")
	}
}

func TestCountsAreFetchedOnRefresh(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	f.countCalls = nil
	_, cmd := m.Update(key("r"))
	drain(t, cmd)
	if len(f.countCalls) != 1 {
		t.Errorf("RepoCounts called %d times on r, want 1", len(f.countCalls))
	}
	if got := f.countCalls[0]; len(got) != 2 || got[0] != "kukv/octoscope" {
		t.Errorf("RepoCounts got %v, want every row's name", got)
	}
}

func TestSidebarListsEveryRepository(t *testing.T) {
	m := sidebarModel(&fakeSource{prs: samplePRs()}, 120)
	view := m.View()
	for _, want := range []string{"kukv/octoscope", "kukv/koto"} {
		if !strings.Contains(view, want) {
			t.Errorf("the sidebar is missing %q:\n%s", want, view)
		}
	}
}

// Design §9: under 100 columns the sidebar folds away and the header keeps
// the name of the repository being shown.
func TestSidebarFoldsAwayWhenNarrow(t *testing.T) {
	m := sidebarModel(&fakeSource{prs: samplePRs()}, 80)
	if strings.Contains(m.View(), "kukv/koto") {
		t.Errorf("the sidebar was drawn at 80 columns:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "kukv/octoscope") {
		t.Errorf("the header lost the current repository:\n%s", m.View())
	}
}

// Every line must fit the terminal: a sidebar that does not subtract itself
// from the table's width runs off the right edge.
func TestEveryLineFitsTheWidth(t *testing.T) {
	for _, w := range []int{80, 100, 120, 160} {
		m := sidebarModel(&fakeSource{prs: samplePRs()}, w)
		for _, line := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(line); got > w {
				t.Errorf("at %d columns a line is %d wide: %q", w, got, line)
			}
		}
	}
}

func TestFocusMovesBetweenPanes(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	if m.focus != paneList {
		t.Fatal("the list did not start focused")
	}
	m, _ = m.Update(key("h"))
	if m.focus != paneSidebar {
		t.Error("h did not move the focus to the sidebar")
	}
	m, _ = m.Update(key("j"))
	if m.selected != 1 {
		t.Errorf("selected = %d, want j to move the sidebar's cursor", m.selected)
	}
	if m.cursors[m.tab] != 0 {
		t.Error("j moved the table's cursor while the sidebar had the focus")
	}
	m, _ = m.Update(key("l"))
	if m.focus != paneList {
		t.Error("l did not move the focus back to the list")
	}
}

func TestMovingTheSidebarFetchesThatRepository(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	f.prRepos = nil
	m, _ = m.Update(key("h"))
	_, cmd := m.Update(key("j")) // onto kukv/koto
	drain(t, cmd)
	if len(f.prRepos) != 1 || f.prRepos[0] != "kukv/koto" {
		t.Errorf("ListPRs got %v, want the row the cursor moved onto", f.prRepos)
	}
}

// The rows the previous repository's answer would fill must not be shown
// under the new one's name.
func TestMovingTheSidebarClearsTheOldList(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j"))
	if strings.Contains(m.View(), "first pr") {
		t.Errorf("the previous repository's rows are still on screen:\n%s", m.View())
	}
}

// A fetch outlives the row that started it. Its answer must not land under
// another repository's name.
func TestAnAnswerForAnotherRepositoryIsDropped(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("j")) // now on kukv/koto
	m, _ = m.Update(prListMsg{repo: "kukv/octoscope", prs: samplePRs()})
	if strings.Contains(m.View(), "first pr") {
		t.Errorf("a stale answer was shown:\n%s", m.View())
	}
}

// The ref that travels to the detail, diff and checks views names the
// repository the row belongs to, not the one the process started in.
func TestSelectedRefNamesTheSelectedRepository(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	m, _ = m.Update(key("h"))
	m, cmd := m.Update(key("j"))
	drain(t, cmd)
	m, _ = m.Update(prListMsg{repo: "kukv/koto", prs: samplePRs()})
	ref, ok := m.SelectedRef()
	if !ok || ref.Repo != "kukv/koto" {
		t.Errorf("ref = %+v, want kukv/koto", ref)
	}
}

// The lookup that names the working directory answers seconds after the
// model was built, and can put a temporary row above the one already loaded.
// What is on screen belongs to the row that was selected, so it must go.
func TestSetCurrentClearsAndRefetches(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120) // showing kukv/octoscope's pull requests
	f.prRepos = nil
	m, cmd := m.SetCurrent("kukv/elsewhere")
	drain(t, cmd)
	if strings.Contains(m.View(), "first pr") {
		t.Errorf("the previous row's rows survived:\n%s", m.View())
	}
	if len(f.prRepos) != 1 || f.prRepos[0] != "kukv/elsewhere" {
		t.Errorf("ListPRs got %v, want the new row", f.prRepos)
	}
}

// SetCurrent rebuilds m.rows from the settings file's strings, which resets
// every row's badge to uncounted. Without a fresh fetchCounts here, a row
// counted before SetCurrent ran stays showing its old numbers forever, or --
// on the ordinary startup path, where RepoCounts usually answers before the
// current-repository lookup does -- every badge stays "—" until r is pressed.
func TestSetCurrentFetchesCounts(t *testing.T) {
	f := &fakeSource{prs: samplePRs()}
	m := sidebarModel(f, 120)
	f.countCalls = nil
	_, cmd := m.SetCurrent("kukv/elsewhere")
	drain(t, cmd)
	if len(f.countCalls) != 1 {
		t.Errorf("RepoCounts called %d times by SetCurrent, want 1", len(f.countCalls))
	}
}

// Design §8: the key bar must fit ja at 80 columns. FitKeyBar guarantees the
// width on its own -- it drops hints until they fit -- so measuring the width
// would assert nothing (see docs: the seven tests that could not fail). What
// is worth holding is which hints survive the drop.
func TestKeyBarKeepsTheEssentialKeysInJapaneseAt80(t *testing.T) {
	i18n.SetLanguage(language.Japanese)
	t.Cleanup(func() { i18n.SetLanguage(language.English) })
	bar := sidebarModel(&fakeSource{prs: samplePRs()}, 80).keyBar()
	for _, want := range []string{
		i18n.T("footer.list.move"),
		i18n.T("footer.list.open"),
		i18n.T("footer.list.pane"),
		i18n.T("footer.list.kind"),
		i18n.T("footer.list.quit"),
	} {
		if !strings.Contains(bar, want) {
			t.Errorf("the key bar dropped %q at ja/80: %q", want, bar)
		}
	}
}

// 20-50 repositories is the realistic size of the list (design §2). The
// sidebar must scroll rather than run off the bottom of the terminal.
func TestSidebarScrollsRatherThanOverflowing(t *testing.T) {
	var many []string
	for i := range 50 {
		many = append(many, fmt.Sprintf("kukv/repo-%02d", i))
	}
	f := &fakeSource{prs: samplePRs()}
	m := sized(New(f, Options{Repositories: many}), 120)
	if got := len(strings.Split(m.View(), "\n")); got > 40 {
		t.Errorf("the view is %d lines tall in a 40-line terminal", got)
	}
	m, _ = m.Update(key("h"))
	for range 49 {
		m, _ = m.Update(key("j"))
	}
	if !strings.Contains(m.View(), "kukv/repo-49") {
		t.Errorf("the last row is off screen:\n%s", m.View())
	}
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
	list := currentModel(f, 120)
	issues, cmd := list.Update(key("tab"))
	issues, _ = issues.Update(cmd())
	empty := loadedModel(&fakeSource{})

	return map[string]string{
		"list_prs":    list.View(),
		"list_issues": issues.View(),
		"empty":       empty.View(),
		"loading":     New(f, Options{}).View(),
	}
}
