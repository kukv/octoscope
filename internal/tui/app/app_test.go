package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/browser"
	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/checks"
	"github.com/kukv/octoscope/internal/tui/detail"
	"github.com/kukv/octoscope/internal/tui/diff"
	"github.com/kukv/octoscope/internal/tui/merge"
	"github.com/kukv/octoscope/internal/tui/repo"
	"github.com/kukv/octoscope/internal/tui/review"
	"github.com/kukv/octoscope/internal/tui/theme"
	"github.com/kukv/octoscope/internal/tui/work"
	"github.com/kukv/octoscope/internal/usecase"
)

// fakeSource satisfies Source. The child views have their own tests; here we
// only exercise the root's routing, so most methods return zero values.
type fakeSource struct {
	work      gh.Work
	prs       []gh.PR
	pr        gh.PR
	prErr     error
	labels    []gh.Label
	files     []gh.FileDiff
	diffErr   error
	checks    gh.Checks
	checksErr error
	// workSections is every board column that was asked for. The board makes
	// one request per column now, so a refresh that dropped a column and one
	// that did not are told apart by which columns were asked for, not by how
	// many requests went out.
	workSections []gh.WorkSection
	workErr      error
	prCalls      int
	prRepos      []string
	issueRepos   []string
	countCalls   [][]string

	searchedFor []string
	found       []gh.RepoCandidate
	seed        []gh.RepoCandidate
	saved       []string

	searchItems []gh.WorkItem
}

func (f *fakeSource) SearchItems(context.Context, string) ([]gh.WorkItem, error) {
	return f.searchItems, nil
}

func (f *fakeSource) ListWorkSection(_ context.Context, s gh.WorkSection) ([]gh.WorkItem, error) {
	f.workSections = append(f.workSections, s)
	return f.work[s], f.workErr
}

// refreshedTheBoard reports whether every column was asked for. A column left
// out stays on screen as it was, which is what a refresh is meant to undo.
func (f *fakeSource) refreshedTheBoard() bool {
	seen := map[gh.WorkSection]bool{}
	for _, s := range f.workSections {
		seen[s] = true
	}
	return len(seen) == gh.WorkSectionCount
}

func (f *fakeSource) ListPRs(_ context.Context, repo string) ([]gh.PR, error) {
	f.prCalls++
	f.prRepos = append(f.prRepos, repo)
	return f.prs, nil
}

func (f *fakeSource) ListIssues(_ context.Context, repo string) ([]gh.Issue, error) {
	f.issueRepos = append(f.issueRepos, repo)
	return nil, nil
}

func (f *fakeSource) RepoName(context.Context) (string, error) { return "kukv/demo", nil }

func (f *fakeSource) RepoCounts(_ context.Context, repos []string) ([]gh.RepoCount, error) {
	f.countCalls = append(f.countCalls, repos)
	return nil, nil
}

func (f *fakeSource) SearchRepos(_ context.Context, query string, _ int) ([]gh.RepoCandidate, error) {
	f.searchedFor = append(f.searchedFor, query)
	return f.found, nil
}

func (f *fakeSource) SeedCandidates(context.Context) ([]gh.RepoCandidate, error) {
	return f.seed, nil
}

func (f *fakeSource) SaveRepositories(repos []string) error {
	f.saved = repos
	return nil
}

func (f *fakeSource) GetItem(_ context.Context, ref gh.ItemRef) (usecase.Item, error) {
	if ref.Kind == gh.ItemIssue {
		return usecase.Item{Kind: gh.ItemIssue}, nil
	}
	pr := f.pr
	return usecase.Item{
		Kind: gh.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
		State: pr.State, Body: pr.Body, URL: pr.URL, Labels: pr.Labels,
		Assignees: pr.Assignees, Comments: pr.Comments, UpdatedAt: pr.UpdatedAt,
		PR: &pr,
	}, f.prErr
}

func (f *fakeSource) OpenWeb(string) error                { return nil }
func (f *fakeSource) AddComment(gh.ItemRef, string) error { return nil }
func (f *fakeSource) SetState(gh.ItemRef, bool) error     { return nil }
func (f *fakeSource) EditLabels(gh.ItemRef, []string, []string) error {
	return nil
}

func (f *fakeSource) EditAssignees(gh.ItemRef, []string, []string) error {
	return nil
}

func (f *fakeSource) ListLabels(context.Context, string) ([]gh.Label, error) {
	return f.labels, nil
}
func (f *fakeSource) ListAssignees(context.Context, string) ([]string, error) { return nil, nil }

func (f *fakeSource) PRDiff(context.Context, string, int) ([]gh.FileDiff, error) {
	return f.files, f.diffErr
}

func (f *fakeSource) PRReviewContext(context.Context, string, int) (gh.ReviewContext, error) {
	return gh.ReviewContext{}, nil
}

func (f *fakeSource) PostLineComment(usecase.ReviewTarget, gh.PendingComment) (string, error) {
	return "", nil
}

func (f *fakeSource) DiscardReview(string) error { return nil }

func (f *fakeSource) SubmitReview(usecase.ReviewTarget, gh.ReviewEvent, string) error { return nil }

func (f *fakeSource) PRChecks(context.Context, string, int) (gh.Checks, error) {
	return f.checks, f.checksErr
}

func (f *fakeSource) JobLog(context.Context, string, int64, bool) ([]gh.LogLine, error) {
	return nil, nil
}

func (f *fakeSource) RerunWorkflow(context.Context, string, int64, gh.RerunScope) error {
	return nil
}

func (f *fakeSource) PRMergeContext(context.Context, string, int) (gh.MergeContext, error) {
	return gh.MergeContext{}, nil
}

func (f *fakeSource) MergePR(string, gh.MergeMethod) error         { return nil }
func (f *fakeSource) EnableAutoMerge(string, gh.MergeMethod) error { return nil }
func (f *fakeSource) DisableAutoMerge(string) error                { return nil }

func newTestModelWith(src Source, opts Options) Model {
	m := New(src, opts)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func newTestModel(opts Options) Model {
	return newTestModelWith(&fakeSource{}, opts)
}

// started is loadedApp with the search tab's own item, so it can be reached
// through app's own key routing without a t.Helper() at every call site.
func started(t *testing.T, width int) Model {
	t.Helper()
	src := &fakeSource{searchItems: []gh.WorkItem{{
		Ref:   gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/demo", Number: 1},
		Title: "a result",
	}}}
	next, cmd := New(src, Options{}).Update(tea.WindowSizeMsg{Width: width, Height: 40})
	return resolve(t, next.(Model), cmd)
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

func press(m Model, k string) Model {
	next, _ := m.Update(key(k))
	return next.(Model)
}

// pressCmd is press for the cases where the returned command matters.
func pressCmd(m Model, k string) (Model, tea.Cmd) {
	next, cmd := m.Update(key(k))
	return next.(Model), cmd
}

// A board every column of which failed has answered, but has never been
// answered with anything: there is no age to report, and the zero time read
// as an age is a hundred thousand days. Starting octoscope with no network
// reaches this.
func TestATabRowReportsNoAgeWhenNothingWasFetched(t *testing.T) {
	f := &fakeSource{workErr: errors.New("gh api: gh: HTTP 502")}
	m := New(f, Options{})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, next.(Model), cmd)

	if !m.work.Summary().Ready {
		t.Fatal("setup: the board has not answered, so the row says nothing anyway")
	}
	if got := m.summary(); got != "" {
		t.Errorf("the tab row dates a board that was never fetched: %q", ansi.Strip(got))
	}
}

// resolve runs cmd and feeds every message it produces back into the model.
// The child views keep their message types unexported, so running their
// commands is the only way to reach a loaded state from here.
func resolve(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = resolve(t, m, c)
		}
		return m
	}
	if msg == nil {
		return m
	}
	next, nextCmd := m.Update(msg)
	m = next.(Model)
	// Spinner ticks command themselves forever; running them would not
	// terminate.
	if _, isTick := msg.(spinner.TickMsg); isTick {
		return m
	}
	return resolve(t, m, nextCmd)
}

// content is the rendered view with its styling stripped, so an assertion
// about a word is not defeated by the escape codes lipgloss puts inside it.
func content(m Model) string { return ansi.Strip(m.View().Content) }

// repoResolvedMsg is the only thing that can settle whether the working
// directory is a repository, so the root has to pass on an empty answer as
// well as a name. Without that, an empty Repos tab says "no repositories
// yet" for as long as the lookup takes.
func TestTheReposTabWaitsForTheLookupBeforeCallingItEmpty(t *testing.T) {
	m := press(newTestModel(Options{}), "2")
	if strings.Contains(content(m), i18n.T("repos.none")) {
		t.Errorf("the tab answered before the lookup did:\n%s", content(m))
	}
	next, _ := m.Update(repoResolvedMsg{})
	if got := content(next.(Model)); !strings.Contains(got, i18n.T("repos.none")) {
		t.Errorf("the tab never answered:\n%s", got)
	}
}

// isQuit reports whether cmd is tea.Quit.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

// TestTheFirstTabFollowsTheFlag covers both halves of the rule: --repo says
// which repository the user came for, so they land on it; without the flag
// there is no repository yet to land on.
func TestTheFirstTabFollowsTheFlag(t *testing.T) {
	if m := New(&fakeSource{}, Options{Repo: "kukv/demo"}); m.tab != tabRepos {
		t.Errorf("with --repo: tab = %d, want tabRepos", m.tab)
	}
	if m := New(&fakeSource{}, Options{}); m.tab != tabWork {
		t.Errorf("without --repo: tab = %d, want tabWork", m.tab)
	}
}

// TestAResolvedRepositoryDoesNotMoveTheUser is the other half: the working
// directory's repository is answered seconds after the board is already on
// screen, and swapping the tab under the user then is not a courtesy.
func TestAResolvedRepositoryDoesNotMoveTheUser(t *testing.T) {
	m := newTestModel(Options{})
	next, _ := m.Update(repoResolvedMsg{name: "kukv/demo"})
	if got := next.(Model); got.tab != tabWork {
		t.Errorf("tab = %d after the repository was resolved, want tabWork", got.tab)
	}
}

func TestThreeShowsTheSearchTab(t *testing.T) {
	t.Parallel()

	m := started(t, 120)
	m = press(m, "3")
	if !strings.Contains(m.View().Content, i18n.T("search.filters")) {
		t.Errorf("3 did not reach the Search tab:\n%s", m.View().Content)
	}
}

// The root acts on q, 1, 2 and 3 before the tabs see them. A query with one
// of those in it must still reach the field.
func TestTypingTheTabKeysIntoTheSearchFieldTypesThem(t *testing.T) {
	t.Parallel()

	m := started(t, 120)
	m = press(m, "3")
	m = press(m, "e")
	for _, key := range []string{"q", "1", "2", "3"} {
		m = press(m, key)
	}
	if !strings.Contains(m.View().Content, "q123") {
		t.Errorf("the tab keys did not reach the field:\n%s", m.View().Content)
	}
}

func TestASearchResultOpensTheDetailView(t *testing.T) {
	t.Parallel()

	m := started(t, 120)
	m = press(m, "3")
	m = press(m, "l")
	m, cmd := pressCmd(m, "enter")
	m = resolve(t, m, cmd)
	if len(m.stack) == 0 {
		t.Error("enter on a result opened nothing")
	}
}

func TestTabKeysSwitchTabs(t *testing.T) {
	// --repo starts on Repos, so 1 is the key that has somewhere to go first.
	m := press(newTestModel(Options{Repo: "kukv/demo"}), "1")
	if m.tab != tabWork {
		t.Errorf("after 1: got %d, want tabWork", m.tab)
	}
	if m = press(m, "2"); m.tab != tabRepos {
		t.Errorf("after 2: got %d, want tabRepos", m.tab)
	}
}

// The Repos tab lists what the settings file holds, so it is there whether or
// not the working directory is a repository.
func TestReposTabExistsWithoutACurrentRepository(t *testing.T) {
	m := press(newTestModel(Options{}), "2")
	if m.tab != tabRepos {
		t.Error("2 did not reach the Repos tab")
	}
	if !strings.Contains(content(m), i18n.T("tab.repos")) {
		t.Errorf("the tab row does not offer Repos:\n%s", content(m))
	}
}

// The lookup's answer names the repository, which is what the list needs to
// put a temporary row at the top of the sidebar.
func TestResolvedRepositoryReachesTheList(t *testing.T) {
	m := newTestModel(Options{})
	next, _ := m.Update(repoResolvedMsg{name: "kukv/demo"})
	if got := next.(Model).repo.Current(); got != "kukv/demo" {
		t.Errorf("the list's current repository = %q, want kukv/demo", got)
	}
}

// The lookup's answer replaces the list's rows, which resets every badge to
// uncounted; without a fresh count fetch, a row counted before the answer
// arrived would be stuck showing its old numbers.
func TestResolvedRepositoryFetchesCounts(t *testing.T) {
	f := &fakeSource{}
	m := newTestModelWith(f, Options{})
	_, cmd := m.Update(repoResolvedMsg{name: "kukv/demo"})
	resolve(t, m, cmd)
	if len(f.countCalls) == 0 {
		t.Error("RepoCounts was not called after the repository was resolved")
	}
}

// TestSidebarMoveAsksListPRsForTheNewRow covers the argument fetchList hands
// down through the routing that reaches this package's Source: moving the
// Repos sidebar's cursor here, not just inside internal/tui/repo, must ask
// ListPRs for the row the cursor landed on.
func TestSidebarMoveAsksListPRsForTheNewRow(t *testing.T) {
	f := &fakeSource{}
	next, cmd := New(f, Options{
		Repo:         "kukv/octoscope",
		Repositories: []string{"kukv/octoscope", "kukv/koto"},
	}).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m := resolve(t, next.(Model), cmd)

	m = press(m, "h") // focus the sidebar
	f.prRepos = nil
	moved, moveCmd := pressCmd(m, "j") // onto kukv/koto
	resolve(t, moved, moveCmd)

	if got := f.prRepos; len(got) != 1 || got[0] != "kukv/koto" {
		t.Errorf("ListPRs got %v, want the row the cursor moved onto", got)
	}
}

// --repo is a statement about this run: its repository is current from the
// start, without waiting for a lookup.
func TestRepoFlagIsCurrentFromTheStart(t *testing.T) {
	m := New(&fakeSource{}, Options{Repo: "kukv/flagged"})
	if got := m.repo.Current(); got != "kukv/flagged" {
		t.Errorf("the list's current repository = %q, want kukv/flagged", got)
	}
	if m.tab != tabRepos {
		t.Error("--repo did not land on the Repos tab")
	}
}

// TestTheFirstWindowSizeStartsTheFetches covers the reason app carries a
// started flag: Init cannot hold the cancel function work.Refresh hands back,
// so the first fetch waits for the first size. A later resize must not cancel
// and restart it.
func TestTheFirstWindowSizeStartsTheFetches(t *testing.T) {
	m := New(&fakeSource{}, Options{Repo: "kukv/demo"})
	// Init asks the terminal for its background colour; that is all it does.
	if m.Init() == nil {
		t.Fatal("Init did not ask the terminal for its background colour")
	}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("the first WindowSizeMsg did not start the fetches")
	}
	if _, cmd = next.(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 24}); cmd != nil {
		t.Error("a resize started the fetches again")
	}
}

// TestALateRepositoryStillGetsTheTerminalWidth guards the row the lookup's
// answer adds to an already-sized list: it must wrap within the width every
// other row already respects, not run off the terminal like an unsized one
// would.
func TestALateRepositoryStillGetsTheTerminalWidth(t *testing.T) {
	const width = 120
	src := &fakeSource{prs: []gh.PR{{
		Number: 1,
		Title: "レンダリングのパイプラインをまるごと置き換える " +
			"refactor with an English clause long enough to run off any screen",
		Author: gh.Author{Login: "a-contributor-with-a-very-long-handle"},
	}}}

	next, cmd := New(src, Options{}).Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m := resolve(t, next.(Model), cmd)
	m = press(m, "2")

	for _, line := range strings.Split(content(m), "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("the list is %d columns wide: %q", w, line)
		}
	}
}

// TestANotFoundRepositoryStartsNoFetch covers the case a lookup answers with
// neither a repository nor a timeout: a directory that is not a repository
// gives the list nothing new to show.
func TestANotFoundRepositoryStartsNoFetch(t *testing.T) {
	m := newTestModel(Options{})
	if _, cmd := m.Update(repoResolvedMsg{}); cmd != nil {
		t.Error("a directory with no repository still started a fetch")
	}
}

// TestTheFirstSizeAsksWhichRepositoryThisIs is the other half: without this,
// the lookup never runs and the list's current repository is never learned.
func TestTheFirstSizeAsksWhichRepositoryThisIs(t *testing.T) {
	m := New(&fakeSource{}, Options{})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("the first size started nothing")
	}
	resolved := resolve(t, next.(Model), cmd)
	if got := resolved.repo.Current(); got != "kukv/demo" {
		t.Errorf("the list's current repository = %q, want kukv/demo", got)
	}
}

func TestOpenDetailMsgShowsTheDetailView(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{
		Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 3},
	})
	m = next.(Model)
	if !m.has(overlayDetail) {
		t.Fatal("the detail view did not open")
	}
	next, _ = m.Update(detail.ClosedMsg{})
	if next.(Model).has(overlayDetail) {
		t.Error("the detail view did not close on ClosedMsg")
	}
}

func TestRepoOpenDetailMsgShowsTheDetailView(t *testing.T) {
	next, _ := newTestModel(Options{Repo: "kukv/demo"}).Update(repo.OpenDetailMsg{
		Ref: gh.ItemRef{Kind: gh.ItemIssue, Number: 7},
	})
	if !next.(Model).has(overlayDetail) {
		t.Error("the detail view did not open")
	}
}

var someRef = gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 3}

func TestDFromTheBoardOpensTheDiffOnItsOwn(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDiffMsg{Ref: someRef})
	got := next.(Model)
	if len(got.stack) != 1 || got.stack[0] != overlayDiff {
		t.Errorf("stack = %v, want just the diff", got.stack)
	}
}

func TestDOpensTheDiffOverTheDetailView(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	next, _ = next.(Model).Update(detail.OpenDiffMsg{Ref: someRef})
	got := next.(Model)
	if len(got.stack) != 2 {
		t.Fatalf("stack = %v, want the detail view with the diff over it", got.stack)
	}
	if got.stack[1] != overlayDiff {
		t.Errorf("the top of the stack is %v, want the diff", got.stack[1])
	}
}

func TestEscTakesTheDiffOffAndLeavesTheDetailView(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	next, _ = next.(Model).Update(detail.OpenDiffMsg{Ref: someRef})
	next, _ = next.(Model).Update(diff.ClosedMsg{})
	got := next.(Model)
	if len(got.stack) != 1 || got.stack[0] != overlayDetail {
		t.Errorf("stack = %v, want just the detail view", got.stack)
	}
}

func TestSFromTheBoardOpensTheChecksOnItsOwn(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenChecksMsg{Ref: someRef})
	got := next.(Model)
	if len(got.stack) != 1 || got.stack[0] != overlayChecks {
		t.Errorf("stack = %v, want just the checks", got.stack)
	}
}

func TestEscTakesTheChecksOffAndLeavesTheDetailView(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	next, _ = next.(Model).Update(detail.OpenChecksMsg{Ref: someRef})
	next, _ = next.(Model).Update(checks.ClosedMsg{})
	got := next.(Model)
	if len(got.stack) != 1 || got.stack[0] != overlayDetail {
		t.Errorf("stack = %v, want just the detail view", got.stack)
	}
}

// TestAStaleClosedMsgDoesNotPopTwice covers two esc presses queued before the
// first ClosedMsg lands: the old showingDetail bool was idempotent to a
// second one, and a stack must be too, or the next legitimate close pops the
// tabs instead of nothing.
func TestAStaleClosedMsgDoesNotPopTwice(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	next, _ = next.(Model).Update(detail.ClosedMsg{})
	next, _ = next.(Model).Update(detail.ClosedMsg{})
	got := next.(Model)
	if len(got.stack) != 0 {
		t.Errorf("stack = %v, want empty after two closes", got.stack)
	}
}

// TestAClosedDiffsFailureIsNotShown mirrors the rule the detail view already
// follows: a request outlives the view that started it, and its failure must
// not drag a closed view's error onto the screen.
func TestAClosedDiffsFailureIsNotShown(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(diff.ErrorMsg{Err: errors.New("boom")})
	if got := next.(Model).errText; got != "" {
		t.Errorf("error screen shows %q for a diff that is not open", got)
	}
}

// TestASubmittedReviewRefreshesTheBoardAndTheReposList guards spec 4.4.2: a
// submitted review must be reflected in the Work board and the Repos list,
// not just in the detail/diff view the reviewer submitted it from.
func TestASubmittedReviewRefreshesTheBoardAndTheReposList(t *testing.T) {
	f := &fakeSource{}
	m := newTestModelWith(f, Options{Repo: "kukv/demo"})

	_, cmd := m.Update(review.SubmittedMsg{})
	if cmd == nil {
		t.Fatal("review.SubmittedMsg produced no command")
	}
	resolve(t, m, cmd)

	if !f.refreshedTheBoard() {
		t.Errorf("the board was not fully refreshed after a submitted review; asked for %v", f.workSections)
	}
	if f.prCalls == 0 {
		t.Error("the Repos list was not refreshed after a submitted review")
	}
}

// TestAMergeRefreshesTheBoardAndTheReposList guards spec 4.4.4: a merged
// pull request must leave the Work board, not only the view it was merged
// from.
func TestAMergeRefreshesTheBoardAndTheReposList(t *testing.T) {
	f := &fakeSource{}
	m := newTestModelWith(f, Options{Repo: "kukv/demo"})

	_, cmd := m.Update(merge.MergedMsg{})
	if cmd == nil {
		t.Fatal("merge.MergedMsg produced no command")
	}
	resolve(t, m, cmd)

	if !f.refreshedTheBoard() {
		t.Errorf("the board was not fully refreshed after a merge; asked for %v", f.workSections)
	}
	if f.prCalls == 0 {
		t.Error("the Repos list was not refreshed after a merge")
	}
}

// TestTheDetailViewGetsTheCurrentSize guards the one place a child is built
// after the WindowSizeMsg has already been seen: without handing it the stored
// size, its viewport would wrap at its own 80-column default forever.
func TestTheDetailViewGetsTheCurrentSize(t *testing.T) {
	src := &fakeSource{pr: gh.PR{Number: 3, Title: "wide", Body: strings.Repeat("word ", 200)}}
	// Not 80: that is the viewport's own default, so a detail view that never
	// heard the size would look right there by accident. Not narrower either,
	// because the detail footer is 73 columns of key bindings and does not
	// clip itself — that is the detail package's business, not the root's.
	const width = 76

	m := New(src, Options{Repo: "kukv/demo"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
	m = next.(Model)

	next, cmd := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 3}})
	m = resolve(t, next.(Model), cmd)

	for _, line := range strings.Split(content(m), "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Fatalf("line is %d columns wide at %d columns: %q", w, width, line)
		}
	}
}

func TestErrorMsgShowsTheErrorScreen(t *testing.T) {
	// The detail view's error only counts while that view is on screen, so its
	// case opens one first.
	for name, tc := range map[string]struct {
		open bool
		msg  tea.Msg
	}{
		"work":   {msg: work.FatalMsg{Err: errors.New("boom")}},
		"repo":   {msg: repo.FatalMsg{Err: errors.New("boom")}},
		"detail": {open: true, msg: detail.ErrorMsg{Err: errors.New("boom")}},
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestModel(Options{Repo: "kukv/demo"})
			if tc.open {
				opened, _ := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
				m = opened.(Model)
			}
			next, _ := m.Update(tc.msg)
			view := content(next.(Model))
			if !strings.Contains(view, "boom") {
				t.Errorf("the error screen does not show the message:\n%s", view)
			}
			if !strings.Contains(view, i18n.T("app.error_title")) {
				t.Errorf("the error screen has no title:\n%s", view)
			}
		})
	}
}

func TestGhNotFoundIsTranslated(t *testing.T) {
	next, _ := newTestModel(Options{Repo: "kukv/demo"}).
		Update(work.FatalMsg{Err: gh.ErrGhNotFound})
	view := content(next.(Model))
	if !strings.Contains(view, i18n.T("error.gh_not_found")) {
		t.Errorf("gh_not_found was not translated:\n%s", view)
	}
}

// Credentials are the user's to fix, and gh's own wording does not say how.
func TestUnauthenticatedIsTranslated(t *testing.T) {
	next, _ := newTestModel(Options{Repo: "kukv/demo"}).
		Update(work.FatalMsg{Err: fmt.Errorf("gh pr list: %w", gh.ErrUnauthenticated)})
	view := content(next.(Model))
	if !strings.Contains(view, i18n.T("error.unauthenticated")) {
		t.Errorf("unauthenticated was not translated:\n%s", view)
	}
}

// TestNoBrowserShowsTheAddress is the whole point of the error carrying the
// URL: a machine with nothing to open it with can still be read off the
// screen and typed in by hand.
func TestNoBrowserShowsTheAddress(t *testing.T) {
	const url = "https://github.com/kukv/octoscope/pull/55"
	next, _ := newTestModel(Options{Repo: "kukv/demo"}).
		Update(work.FatalMsg{Err: &browser.NoneError{URL: url}})
	view := content(next.(Model))
	if !strings.Contains(view, url) {
		t.Errorf("the error screen does not carry the address:\n%s", view)
	}
}

func TestErrorScreenKeysQuit(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			next, _ := newTestModel(Options{Repo: "kukv/demo"}).
				Update(work.FatalMsg{Err: errors.New("boom")})
			_, cmd := next.(Model).Update(key(k))
			if !isQuit(cmd) {
				t.Errorf("%s did not quit from the error screen", k)
			}
		})
	}
}

// TestEscGoesBackFromAnErrorOverAnOverlay covers the diff view failing on a
// pull request too large for gh: esc must return to what was underneath
// rather than quitting the whole session (the bug the user hit).
func TestEscGoesBackFromAnErrorOverAnOverlay(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDiffMsg{Ref: someRef})
	m = next.(Model)
	next, _ = m.Update(diff.ErrorMsg{Err: errors.New("boom")})
	m = next.(Model)
	if m.errText == "" {
		t.Fatal("the error screen did not show")
	}

	next, cmd := m.Update(key("esc"))
	m = next.(Model)
	if isQuit(cmd) {
		t.Fatal("esc quit the app instead of going back")
	}
	if m.errText != "" {
		t.Error("esc did not clear the error")
	}
	if len(m.stack) != 0 {
		t.Errorf("stack = %v, want empty: the failed diff view must come off it", m.stack)
	}
}

// TestEscGoesBackToTheDetailViewFromAnErrorOverIt is the deeper case: a diff
// opened over the detail view fails, and esc must land back on the detail
// view, not on the tabs.
func TestEscGoesBackToTheDetailViewFromAnErrorOverIt(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	m = next.(Model)
	next, _ = m.Update(detail.OpenDiffMsg{Ref: someRef})
	m = next.(Model)
	next, _ = m.Update(diff.ErrorMsg{Err: errors.New("boom")})
	m = next.(Model)

	next, _ = m.Update(key("esc"))
	m = next.(Model)
	if m.errText != "" {
		t.Error("esc did not clear the error")
	}
	if len(m.stack) != 1 || m.stack[0] != overlayDetail {
		t.Errorf("stack = %v, want just the detail view", m.stack)
	}
}

// TestEscLeavesAnUnrelatedOverlayStanding covers the reachable path a tab's
// failure takes while an overlay is open: submit a review from the diff, the
// root refreshes the board, and the board finds gh gone with the diff still
// on top. That failure belongs to neither overlay on the stack, so esc must
// clear it without discarding a diff that never failed.
func TestEscLeavesAnUnrelatedOverlayStanding(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: someRef})
	m = next.(Model)
	next, _ = m.Update(detail.OpenDiffMsg{Ref: someRef})
	m = next.(Model)
	next, _ = m.Update(work.FatalMsg{Err: gh.ErrGhNotFound})
	m = next.(Model)
	if m.errText == "" {
		t.Fatal("the error screen did not show")
	}

	next, _ = m.Update(key("esc"))
	m = next.(Model)
	if m.errText != "" {
		t.Error("esc did not clear the error")
	}
	if len(m.stack) != 2 || m.stack[1] != overlayDiff {
		t.Errorf("stack = %v, want [detail diff] still: esc must not discard a diff that never failed", m.stack)
	}
}

// TestEscStillQuitsWithNoOverlay is the start-up case: gh not being on PATH
// fails before anything is on the stack, and there is nowhere for esc to go
// back to, so it must keep quitting like q does.
func TestEscStillQuitsWithNoOverlay(t *testing.T) {
	next, _ := newTestModel(Options{Repo: "kukv/demo"}).
		Update(work.FatalMsg{Err: gh.ErrGhNotFound})
	m := next.(Model)
	if len(m.stack) != 0 {
		t.Fatalf("stack = %v, want empty for this case", m.stack)
	}
	_, cmd := m.Update(key("esc"))
	if !isQuit(cmd) {
		t.Error("esc did not quit with nothing to go back to")
	}
}

// TestErrorScreenKeyBarNamesWhatIsAvailable guards the footer: it must say
// esc:back only when there is something to go back to.
func TestErrorScreenKeyBarNamesWhatIsAvailable(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDiffMsg{Ref: someRef})
	next, _ = next.(Model).Update(diff.ErrorMsg{Err: errors.New("boom")})

	view := content(next.(Model))
	if !strings.Contains(view, i18n.T("footer.error.esc")) {
		t.Errorf("key bar does not offer esc:back with an overlay open:\n%s", view)
	}

	noOverlay, _ := newTestModel(Options{Repo: "kukv/demo"}).
		Update(work.FatalMsg{Err: errors.New("boom")})
	view = content(noOverlay.(Model))
	if strings.Contains(view, i18n.T("footer.error.esc")) {
		t.Errorf("key bar offers esc:back with nothing to go back to:\n%s", view)
	}
	if !strings.Contains(view, i18n.T("footer.error.quit")) {
		t.Errorf("key bar does not offer q:quit:\n%s", view)
	}
}

func TestQQuitsOnTheTabs(t *testing.T) {
	_, cmd := pressCmd(newTestModel(Options{Repo: "kukv/demo"}), "q")
	if !isQuit(cmd) {
		t.Error("q did not quit the app")
	}
}

// TestQGoesBackInTheDetailView guards the one place q does not mean quit: the
// detail view answers it with ClosedMsg, the way esc does.
func TestQGoesBackInTheDetailView(t *testing.T) {
	for _, k := range []string{"q", "esc"} {
		t.Run(k, func(t *testing.T) {
			m := newTestModel(Options{Repo: "kukv/demo"})
			next, _ := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
			m, cmd := pressCmd(next.(Model), k)
			if isQuit(cmd) {
				t.Fatalf("%s quit the app instead of leaving the detail view", k)
			}
			m = resolve(t, m, cmd)
			if m.has(overlayDetail) {
				t.Errorf("%s did not leave the detail view", k)
			}
		})
	}
}

// TestCtrlCQuitsWhileTheDetailViewIsBusy is the reason app tests ctrl+c before
// delegating. The detail view swallows every key while an operation is in
// flight, so a ctrl+c routed through it would leave the app unquittable.
func TestCtrlCQuitsWhileTheDetailViewIsBusy(t *testing.T) {
	tests := map[string]func(t *testing.T, m Model) Model{
		"posting a comment": func(t *testing.T, m Model) Model {
			m = press(m, "c")
			next, _ := m.Update(tea.KeyPressMsg{Code: 'h', Text: "hi"})
			m, cmd := pressCmd(next.(Model), "ctrl+s")
			if cmd == nil {
				t.Fatal("ctrl+s did not post the comment")
			}
			return m
		},
		"closing the item": func(t *testing.T, m Model) Model {
			m = press(m, "x")
			m, cmd := pressCmd(m, "y")
			if cmd == nil {
				t.Fatal("y did not start the state change")
			}
			return m
		},
		"loading picker candidates": func(_ *testing.T, m Model) Model {
			return press(m, "l")
		},
		"applying a picker": func(t *testing.T, m Model) Model {
			m, cmd := pressCmd(m, "l")
			m = resolve(t, m, cmd)
			m = press(m, "space")
			m, cmd = pressCmd(m, "enter")
			if cmd == nil {
				t.Fatal("enter did not apply the picker")
			}
			return m
		},
	}

	for name, busy := range tests {
		t.Run(name, func(t *testing.T) {
			src := &fakeSource{
				pr:     gh.PR{Number: 1, Title: "a pr", State: gh.StateOpen},
				labels: []gh.Label{{Name: "bug", Color: "d73a4a"}},
			}
			m := newTestModelWith(src, Options{Repo: "kukv/demo"})
			next, cmd := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
			m = resolve(t, next.(Model), cmd)

			m = busy(t, m)
			if _, cmd := m.Update(key("ctrl+c")); !isQuit(cmd) {
				t.Error("ctrl+c did not quit while the detail view was busy")
			}
		})
	}
}

// TestALateRepoMessageIsNotDropped covers the seam between the views: r on the
// Repos tab, then a card opened before the refresh returns. The late list must
// still reach repo, or its spinner is stuck when the user comes back.
func TestALateRepoMessageIsNotDropped(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	m = press(m, "2")

	m, refresh := pressCmd(m, "r")
	if refresh == nil {
		t.Fatal("r did not refresh the list")
	}

	next, _ := m.Update(repo.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
	m = next.(Model)

	m = resolve(t, m, refresh) // the list arrives while the detail view is front

	// The list's own message must not touch the detail view, which is still
	// fetching and must still say so.
	if !strings.Contains(content(m), i18n.T("common.loading")) {
		t.Errorf("the detail view lost its spinner to the list's message:\n%s", content(m))
	}

	next, _ = m.Update(detail.ClosedMsg{})
	view := content(next.(Model))
	if strings.Contains(view, i18n.T("common.loading")) {
		t.Errorf("the list is still loading after its data arrived:\n%s", view)
	}
	if !strings.Contains(view, i18n.T("list.no_open_prs")) {
		t.Errorf("the list did not render its data:\n%s", view)
	}
}

func TestKeysReachTheTabUnderneath(t *testing.T) {
	src := &fakeSource{work: gh.Work{
		gh.SectionReviewRequested: {{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}, Title: "first"}},
	}}
	m := New(src, Options{}) // no --repo: the board is the first tab
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, next.(Model), cmd)

	if _, ok := m.work.SelectedRef(); !ok {
		t.Fatal("the board has no selection to move")
	}
	// l moves the board's cursor to the next column, which is empty here.
	m = press(m, "l")
	if _, ok := m.work.SelectedRef(); ok {
		t.Error("the Work board did not receive the key")
	}
}

// TestEnterOnTheBoardOpensTheDetailView walks the whole path a user takes:
// the key reaches the board, the board asks for the detail view, and the root
// puts it on screen.
func TestEnterOnTheBoardOpensTheDetailView(t *testing.T) {
	src := &fakeSource{
		work: gh.Work{
			gh.SectionReviewRequested: {{
				Ref:   gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 41},
				Title: "add the work board",
			}},
		},
		pr: gh.PR{Number: 41, Title: "add the work board", State: gh.StateOpen},
	}
	m := New(src, Options{}) // no --repo: the board is the first tab
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, next.(Model), cmd)

	m, cmd = pressCmd(m, "enter")
	m = resolve(t, m, cmd)
	if !m.has(overlayDetail) {
		t.Fatal("enter on the board did not open the detail view")
	}
	if !strings.Contains(content(m), "add the work board") {
		t.Errorf("the detail view did not fetch the selected item:\n%s", content(m))
	}
}

func TestTheTabRowMarksTheActiveTab(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	view := content(m)
	for _, want := range []string{i18n.T("tab.work"), i18n.T("tab.repos")} {
		if !strings.Contains(view, want) {
			t.Errorf("the tab row is missing %q:\n%s", want, view)
		}
	}
	i18n.AssertNoUnresolvedIDs(t, view)
}

// A settings file that cannot be read must not be silent: the user is running
// with defaults and has no other way to find out.
func TestTheTabRowSaysTheSettingsFileCouldNotBeRead(t *testing.T) {
	m := New(&fakeSource{}, Options{ConfigError: "parse config.yaml: yaml: line 1: did not find expected node content"})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	view := next.(Model).View().Content
	if !strings.Contains(view, i18n.T("tab.config_unreadable")) {
		t.Error("the tab row does not report the unreadable settings file")
	}
	i18n.AssertNoUnresolvedIDs(t, view)
}

func TestTheTabRowIsQuietWhenTheSettingsFileIsFine(t *testing.T) {
	m := New(&fakeSource{}, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	if strings.Contains(next.(Model).View().Content, i18n.T("tab.config_unreadable")) {
		t.Error("the tab row reports an unreadable settings file that was fine")
	}
}

func TestTheDetailViewHasNoTabRow(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
	if strings.Contains(content(next.(Model)), i18n.T("tab.repos")) {
		t.Error("the detail view is drawn under the tab row")
	}
}

// overlongTitle is wider than any terminal the width test uses, in both
// scripts, so the fixture reaches the edge at every width instead of leaving
// the truncation untested.
const overlongTitle = "レンダリングのパイプラインをまるごと置き換える " +
	"refactor that nobody asked for"

// overlongSource fills every tab with content that overflows on its own.
func overlongSource() *fakeSource {
	return &fakeSource{
		work: gh.Work{
			gh.SectionReviewRequested: {{
				Ref:   gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/a-repository-nobody-would-name-this-way", Number: 1},
				Title: overlongTitle,
			}},
		},
		prs: []gh.PR{{
			Number: 1, Title: overlongTitle,
			Author: gh.Author{Login: "a-contributor-with-a-very-long-handle"},
		}},
		pr: gh.PR{Number: 1, Title: overlongTitle, State: gh.StateOpen},
	}
}

// renderEveryScreen renders every screen the root can show at width.
func renderEveryScreen(t *testing.T, width int) map[string]string {
	t.Helper()
	size := tea.WindowSizeMsg{Width: width, Height: 40}

	src := overlongSource()
	// --repo opens on the Repos tab; 1 is what reaches the board from there.
	next, cmd := New(src, Options{Repo: "kukv/demo"}).Update(size)
	reposM := resolve(t, next.(Model), cmd)

	next, cmd = reposM.Update(key("1"))
	board := resolve(t, next.(Model), cmd)

	item, cmd := reposM.Update(key("enter"))
	item = resolve(t, item.(Model), cmd)

	failed, _ := board.Update(work.FatalMsg{Err: errors.New(overlongTitle)})

	// An error tied to an overlay draws a different key bar (esc:back, not
	// just q:quit) from the board/Repos-list case above, and that bar's IDs
	// (footer.error.esc in particular) are only ever exercised through this
	// state — nothing else in this function opens an overlay and fails it.
	overlayFailed, cmd := board.Update(work.OpenDiffMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 1}})
	overlayFailed = resolve(t, overlayFailed.(Model), cmd)
	overlayFailed, _ = overlayFailed.(Model).Update(diff.ErrorMsg{Err: errors.New(overlongTitle)})

	noRepo, cmd := New(src, Options{}).Update(size)
	noRepo = resolve(t, noRepo.(Model), cmd)

	return map[string]string{
		"work":          content(board),
		"repos":         content(reposM),
		"detail":        content(item.(Model)),
		"error":         content(failed.(Model)),
		"error_overlay": content(overlayFailed.(Model)),
		"no_repo":       content(noRepo.(Model)),
	}
}

// TestNoLineExceedsTheTerminalWidth guards spec §6.4 across every screen the
// root can show. A Japanese character occupies two columns, so a line that
// fits in English can still run off the screen in Japanese.
func TestNoLineExceedsTheTerminalWidth(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{50, 80, 100, 120} {
			for name, view := range renderEveryScreen(t, width) {
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

// TestAClosedDetailViewDoesNotShowItsError covers the fetch that fails after
// the user already left: nothing is on screen for that view any more, so the
// failure has nowhere to go but away.
func TestAClosedDetailViewDoesNotShowItsError(t *testing.T) {
	m := newTestModel(Options{Repo: "kukv/demo"})
	next, _ := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
	m, cmd := pressCmd(next.(Model), "q")
	m = resolve(t, m, cmd)
	if m.has(overlayDetail) {
		t.Fatal("q did not leave the detail view")
	}

	next, _ = m.Update(detail.ErrorMsg{Err: errors.New("boom")})
	if view := content(next.(Model)); strings.Contains(view, "boom") {
		t.Errorf("a closed detail view's error took over the screen:\n%s", view)
	}
}

// TestAStaleDetailErrorDoesNotReplaceTheOpenOne covers the same failure
// arriving after the user opened a second item: the error belongs to the
// request the user abandoned, while the view now on screen has its own in
// flight.
func TestAStaleDetailErrorDoesNotReplaceTheOpenOne(t *testing.T) {
	m := newTestModelWith(&fakeSource{prErr: errors.New("boom")}, Options{Repo: "kukv/demo"})

	next, first := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
	m, cmd := pressCmd(next.(Model), "q")
	m = resolve(t, m, cmd)

	next, second := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 2}})
	m = next.(Model)

	m = resolve(t, m, first) // the first item's failure lands on the second
	if view := content(m); strings.Contains(view, "boom") {
		t.Errorf("a stale detail error took over the screen:\n%s", view)
	}

	// The control: the shown item's own failure still reaches the error screen.
	m = resolve(t, m, second)
	if view := content(m); !strings.Contains(view, "boom") {
		t.Errorf("the open item's own error was suppressed too:\n%s", view)
	}
}

// TestNoUnresolvedIDsInTheRootViews guards spec §6.5: a message ID the code
// asks for but the catalog of the active language does not carry renders as
// "!the.id" rather than failing anywhere else.
func TestNoUnresolvedIDsInTheRootViews(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for name, view := range renderEveryScreen(t, 80) {
			t.Run(lang.String()+"/"+name, func(t *testing.T) {
				i18n.AssertNoUnresolvedIDs(t, view)
			})
		}
	}
}

// TestTheTerminalBackgroundReachesThePalette covers the one thing the root
// does with a colour: the palette cannot assume a background, and this is the
// only message that reports the real one.
func TestTheTerminalBackgroundReachesThePalette(t *testing.T) {
	t.Cleanup(func() { theme.SetDark(true) })

	m := newTestModel(Options{})
	onDark := theme.Dim().Render("x")

	if _, cmd := m.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")}); cmd != nil {
		t.Error("learning the background started work")
	}
	if theme.Dim().Render("x") == onDark {
		t.Error("a light terminal is still drawn with the dark palette")
	}
}

// TestASlowLookupSaysSoInsteadOfStayingSilent is the difference between the
// two ways the current repository can stay unknown. `gh repo view` reaches
// the API and a cold one has been measured at over six seconds; treating
// that the same as "this directory is not a repository" leaves no trace on
// screen, and the missing current repository gets blamed on whatever else
// changed that day.
func TestASlowLookupSaysSoInsteadOfStayingSilent(t *testing.T) {
	m := newTestModel(Options{})

	quiet, _ := m.Update(repoResolvedMsg{})
	if got := content(quiet.(Model)); strings.Contains(got, i18n.T("tab.repo_lookup_timeout")) {
		t.Errorf("a directory with no repository is reported as a timeout: %q", got)
	}

	slow, _ := m.Update(repoResolvedMsg{timedOut: true})
	if got := content(slow.(Model)); !strings.Contains(got, i18n.T("tab.repo_lookup_timeout")) {
		t.Errorf("a lookup that timed out says nothing: %q", got)
	}
}

// TestALookupThatRanOutOfTimeIsToldApartFromOneThatAnswered covers the seam
// the test above cannot: exec reports a killed subprocess as "signal: killed",
// so a check for context.DeadlineExceeded on the returned error never fires.
func TestALookupThatRanOutOfTimeIsToldApartFromOneThatAnswered(t *testing.T) {
	expired, cancel := context.WithCancel(context.Background())
	cancel()

	cases := map[string]struct {
		ctx  context.Context
		name string
		err  error
		want repoResolvedMsg
	}{
		"a repository": {
			context.Background(), "kukv/octoscope", nil,
			repoResolvedMsg{name: "kukv/octoscope"},
		},
		"none here": {
			context.Background(), "", errors.New("no repository in this directory"),
			repoResolvedMsg{},
		},
		"out of time": {
			expired, "", errors.New("signal: killed"),
			repoResolvedMsg{timedOut: true},
		},
	}
	for name, c := range cases {
		if got := resolved(c.ctx, c.name, c.err); got != c.want {
			t.Errorf("%s: resolved = %+v, want %+v", name, got, c.want)
		}
	}
}

// Someone who set default_tab: repos wants the Repos tab even when the
// repository is found from the working directory rather than named on the
// command line -- but that move waits for the lookup to answer.
func TestDefaultReposWaitsForTheRepositoryToBeFound(t *testing.T) {
	m := New(&fakeSource{}, Options{DefaultRepos: true})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)

	if m.tab != tabWork {
		t.Fatalf("tab before the lookup answered = %v, want tabWork", m.tab)
	}

	next, _ = m.Update(repoResolvedMsg{name: "kukv/octoscope"})
	if got := next.(Model).tab; got != tabRepos {
		t.Errorf("tab after the repository was found = %v, want tabRepos", got)
	}
}

// Without the setting, a repository found from the working directory does not
// move the user off the board they started on.
func TestAFoundRepositoryDoesNotMoveTheUserByItself(t *testing.T) {
	m := New(&fakeSource{}, Options{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.(Model).Update(repoResolvedMsg{name: "kukv/octoscope"})

	if got := next.(Model).tab; got != tabWork {
		t.Errorf("tab = %v, want tabWork", got)
	}
}

// The setting must not pull the user back after they have moved: it chooses
// where the run starts, not where it stays.
func TestDefaultReposDoesNotPullTheUserBackAfterTheyMove(t *testing.T) {
	m := New(&fakeSource{}, Options{DefaultRepos: true})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	next, _ = next.(Model).Update(repoResolvedMsg{name: "kukv/octoscope"})
	onWork := press(next.(Model), "1")

	after, _ := onWork.Update(repoResolvedMsg{name: "kukv/octoscope"})
	if got := after.(Model).tab; got != tabWork {
		t.Errorf("tab = %v, want tabWork", got)
	}
}
