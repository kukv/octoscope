package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/detail"
	"github.com/kukv/octoscope/internal/tui/repo"
	"github.com/kukv/octoscope/internal/tui/theme"
	"github.com/kukv/octoscope/internal/tui/work"
)

// fakeSource satisfies Source. The child views have their own tests; here we
// only exercise the root's routing, so most methods return zero values.
type fakeSource struct {
	work   gh.Work
	prs    []gh.PR
	pr     gh.PR
	prErr  error
	labels []gh.Label
}

func (f *fakeSource) ListWork(context.Context) (gh.Work, error) { return f.work, nil }

func (f *fakeSource) ListPRs(context.Context) ([]gh.PR, error)       { return f.prs, nil }
func (f *fakeSource) ListIssues(context.Context) ([]gh.Issue, error) { return nil, nil }
func (f *fakeSource) RepoName(context.Context) (string, error)       { return "kukv/demo", nil }

func (f *fakeSource) GetPR(context.Context, string, int) (gh.PR, error) { return f.pr, f.prErr }
func (f *fakeSource) GetIssue(context.Context, string, int) (gh.Issue, error) {
	return gh.Issue{}, nil
}
func (f *fakeSource) OpenPRWeb(string, int) error               { return nil }
func (f *fakeSource) OpenIssueWeb(string, int) error            { return nil }
func (f *fakeSource) AddPRComment(string, int, string) error    { return nil }
func (f *fakeSource) AddIssueComment(string, int, string) error { return nil }
func (f *fakeSource) ClosePR(string, int) error                 { return nil }
func (f *fakeSource) ReopenPR(string, int) error                { return nil }
func (f *fakeSource) CloseIssue(string, int) error              { return nil }
func (f *fakeSource) ReopenIssue(string, int) error             { return nil }
func (f *fakeSource) ListLabels(context.Context, string) ([]gh.Label, error) {
	return f.labels, nil
}
func (f *fakeSource) ListAssignees(context.Context, string) ([]string, error) { return nil, nil }
func (f *fakeSource) EditPRLabels(string, int, []string, []string) error      { return nil }
func (f *fakeSource) EditIssueLabels(string, int, []string, []string) error   { return nil }
func (f *fakeSource) EditPRAssignees(string, int, []string, []string) error   { return nil }
func (f *fakeSource) EditIssueAssignees(string, int, []string, []string) error {
	return nil
}

func newTestModelWith(src Source, opts Options) Model {
	m := New(src, opts)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return next.(Model)
}

func newTestModel(opts Options) Model {
	return newTestModelWith(&fakeSource{}, opts)
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

// isQuit reports whether cmd is tea.Quit.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestStartsOnTheWorkTab(t *testing.T) {
	if m := newTestModel(Options{HasRepo: true}); m.tab != tabWork {
		t.Errorf("tab: got %v, want tabWork", m.tab)
	}
}

func TestTabKeysSwitchTabs(t *testing.T) {
	m := press(newTestModel(Options{HasRepo: true}), "2")
	if m.tab != tabRepos {
		t.Errorf("after 2: got %v, want tabRepos", m.tab)
	}
	if m = press(m, "1"); m.tab != tabWork {
		t.Errorf("after 1: got %v, want tabWork", m.tab)
	}
}

// TestReposTabIsUnreachableWithoutARepository guards spec 3.4: with neither
// --repo nor a git remote there is nothing for the Repos tab to show, so the
// app stays on Work rather than surfacing gh's "no git remotes found".
func TestReposTabIsUnreachableWithoutARepository(t *testing.T) {
	m := press(newTestModel(Options{HasRepo: false}), "2")
	if m.tab != tabWork {
		t.Errorf("tab: got %v, want it to stay on tabWork", m.tab)
	}
	if strings.Contains(content(m), i18n.T("tab.repos")) {
		t.Error("the Repos tab is offered even though no repository is known")
	}
}

// TestTheFirstWindowSizeStartsTheFetches covers the reason app carries a
// started flag: Init cannot hold the cancel function work.Refresh hands back,
// so the first fetch waits for the first size. A later resize must not cancel
// and restart it.
func TestTheFirstWindowSizeStartsTheFetches(t *testing.T) {
	m := New(&fakeSource{}, Options{HasRepo: true})
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

// TestTheReposTabAppearsWhenTheRepositoryIsResolved covers what replaced the
// blocking lookup main used to do before the UI started: the tab is not
// offered until the answer arrives, and arriving is what starts its fetches.
func TestTheReposTabAppearsWhenTheRepositoryIsResolved(t *testing.T) {
	m := newTestModel(Options{}) // no --repo: the answer is not known yet
	if strings.Contains(content(m), i18n.T("tab.repos")) {
		t.Error("the Repos tab is offered before the repository is known")
	}

	next, cmd := m.Update(repoResolvedMsg{found: true})
	m = next.(Model)
	if !strings.Contains(content(m), i18n.T("tab.repos")) {
		t.Error("the Repos tab did not appear once the repository was known")
	}
	if cmd == nil {
		t.Error("the repository list was never asked to fetch anything")
	}
	if press(m, "2").tab != tabRepos {
		t.Error("the Repos tab cannot be reached even though it is offered")
	}
}

// TestALateRepositoryStillGetsTheTerminalWidth is the case the asynchronous
// lookup created: the list is not part of the broadcast until the answer
// arrives, so it never saw the size everything else was given. An unsized
// list clips nothing, and every long title runs off the terminal until the
// user happens to resize the window.
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

func TestNoRepositoryLeavesTheReposTabOff(t *testing.T) {
	m := newTestModel(Options{})
	next, cmd := m.Update(repoResolvedMsg{found: false})
	if cmd != nil {
		t.Error("a directory with no repository still started a fetch")
	}
	if strings.Contains(content(next.(Model)), i18n.T("tab.repos")) {
		t.Error("the Repos tab is offered for a directory with no repository")
	}
}

// TestTheFirstSizeAsksWhetherThereIsARepository is the other half: without
// this, the answer never arrives and the tab never appears.
func TestTheFirstSizeAsksWhetherThereIsARepository(t *testing.T) {
	m := New(&fakeSource{}, Options{})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("the first size started nothing")
	}
	resolved := resolve(t, next.(Model), cmd)
	if !resolved.opts.HasRepo {
		t.Error("the lookup ran but its answer did not reach the model")
	}
}

func TestOpenDetailMsgShowsTheDetailView(t *testing.T) {
	m := newTestModel(Options{HasRepo: true})
	next, _ := m.Update(work.OpenDetailMsg{
		Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 3},
	})
	m = next.(Model)
	if !m.showingDetail {
		t.Fatal("the detail view did not open")
	}
	next, _ = m.Update(detail.ClosedMsg{})
	if next.(Model).showingDetail {
		t.Error("the detail view did not close on ClosedMsg")
	}
}

func TestRepoOpenDetailMsgShowsTheDetailView(t *testing.T) {
	next, _ := newTestModel(Options{HasRepo: true}).Update(repo.OpenDetailMsg{
		Ref: gh.ItemRef{Kind: gh.ItemIssue, Number: 7},
	})
	if !next.(Model).showingDetail {
		t.Error("the detail view did not open")
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

	m := New(src, Options{HasRepo: true})
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
		"work":   {msg: work.ErrorMsg{Err: errors.New("boom")}},
		"repo":   {msg: repo.ErrorMsg{Err: errors.New("boom")}},
		"detail": {open: true, msg: detail.ErrorMsg{Err: errors.New("boom")}},
	} {
		t.Run(name, func(t *testing.T) {
			m := newTestModel(Options{HasRepo: true})
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
	next, _ := newTestModel(Options{HasRepo: true}).
		Update(work.ErrorMsg{Err: gh.ErrGhNotFound})
	view := content(next.(Model))
	if !strings.Contains(view, i18n.T("error.gh_not_found")) {
		t.Errorf("gh_not_found was not translated:\n%s", view)
	}
}

func TestErrorScreenKeysQuit(t *testing.T) {
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			next, _ := newTestModel(Options{HasRepo: true}).
				Update(work.ErrorMsg{Err: errors.New("boom")})
			_, cmd := next.(Model).Update(key(k))
			if !isQuit(cmd) {
				t.Errorf("%s did not quit from the error screen", k)
			}
		})
	}
}

func TestQQuitsOnTheTabs(t *testing.T) {
	_, cmd := pressCmd(newTestModel(Options{HasRepo: true}), "q")
	if !isQuit(cmd) {
		t.Error("q did not quit the app")
	}
}

// TestQGoesBackInTheDetailView guards the one place q does not mean quit: the
// detail view answers it with ClosedMsg, the way esc does.
func TestQGoesBackInTheDetailView(t *testing.T) {
	for _, k := range []string{"q", "esc"} {
		t.Run(k, func(t *testing.T) {
			m := newTestModel(Options{HasRepo: true})
			next, _ := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
			m, cmd := pressCmd(next.(Model), k)
			if isQuit(cmd) {
				t.Fatalf("%s quit the app instead of leaving the detail view", k)
			}
			m = resolve(t, m, cmd)
			if m.showingDetail {
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
			m := newTestModelWith(src, Options{HasRepo: true})
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
	m := newTestModel(Options{HasRepo: true})
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
	m := New(src, Options{HasRepo: true})
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
	m := New(src, Options{HasRepo: true})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, next.(Model), cmd)

	m, cmd = pressCmd(m, "enter")
	m = resolve(t, m, cmd)
	if !m.showingDetail {
		t.Fatal("enter on the board did not open the detail view")
	}
	if !strings.Contains(content(m), "add the work board") {
		t.Errorf("the detail view did not fetch the selected item:\n%s", content(m))
	}
}

func TestTheTabRowMarksTheActiveTab(t *testing.T) {
	m := newTestModel(Options{HasRepo: true})
	view := content(m)
	for _, want := range []string{i18n.T("tab.work"), i18n.T("tab.repos")} {
		if !strings.Contains(view, want) {
			t.Errorf("the tab row is missing %q:\n%s", want, view)
		}
	}
	i18n.AssertNoUnresolvedIDs(t, view)
}

func TestTheDetailViewHasNoTabRow(t *testing.T) {
	m := newTestModel(Options{HasRepo: true})
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
	next, cmd := New(src, Options{HasRepo: true}).Update(size)
	board := resolve(t, next.(Model), cmd)

	repos, cmd := board.Update(key("2"))
	repos = resolve(t, repos.(Model), cmd)

	item, cmd := repos.Update(key("enter"))
	item = resolve(t, item.(Model), cmd)

	failed, _ := board.Update(work.ErrorMsg{Err: errors.New(overlongTitle)})

	noRepo, cmd := New(src, Options{}).Update(size)
	noRepo = resolve(t, noRepo.(Model), cmd)

	return map[string]string{
		"work":    content(board),
		"repos":   content(repos.(Model)),
		"detail":  content(item.(Model)),
		"error":   content(failed.(Model)),
		"no_repo": content(noRepo.(Model)),
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
	m := newTestModel(Options{HasRepo: true})
	next, _ := m.Update(work.OpenDetailMsg{Ref: gh.ItemRef{Kind: gh.ItemPR, Number: 1}})
	m, cmd := pressCmd(next.(Model), "q")
	m = resolve(t, m, cmd)
	if m.showingDetail {
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
	m := newTestModelWith(&fakeSource{prErr: errors.New("boom")}, Options{HasRepo: true})

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

// TestASlowLookupSaysSoInsteadOfDroppingTheTab is the difference between the
// two ways the Repos tab can be missing. `gh repo view` reaches the API and a
// cold one has been measured at over six seconds; treating that the same as
// "this directory is not a repository" makes the tab vanish for a reason the
// screen never gives, and the disappearance gets blamed on whatever else
// changed that day.
func TestASlowLookupSaysSoInsteadOfDroppingTheTab(t *testing.T) {
	m := newTestModel(Options{})

	quiet, _ := m.Update(repoResolvedMsg{found: false})
	if got := content(quiet.(Model)); strings.Contains(got, i18n.T("tab.repo_lookup_timeout")) {
		t.Errorf("a directory with no repository is reported as a timeout: %q", got)
	}

	slow, _ := m.Update(repoResolvedMsg{found: false, timedOut: true})
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
		"a repository": {context.Background(), "kukv/octoscope", nil, repoResolvedMsg{found: true}},
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
