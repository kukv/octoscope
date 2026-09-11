package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	query string
	items []gh.WorkItem
	err   error

	labels    []gh.Label
	labelRepo string

	users      []string
	authorRepo string
}

func (f *fakeSource) SearchItems(_ context.Context, query string) ([]gh.WorkItem, error) {
	f.query = query
	return f.items, f.err
}

func (f *fakeSource) OpenWeb(string) error { return nil }

func (f *fakeSource) ListLabels(_ context.Context, repo string) ([]gh.Label, error) {
	f.labelRepo = repo
	return f.labels, nil
}

func (f *fakeSource) ListAssignees(_ context.Context, repo string) ([]string, error) {
	f.authorRepo = repo
	return f.users, nil
}

// resolve runs a command the model handed back and feeds its message in, the
// way Bubble Tea would. A batch is expanded and each of its commands run in
// turn, since Bubble Tea itself never hands Update a tea.BatchMsg.
func resolve(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		t.Fatal("no command to run")
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = resolve(t, m, c)
		}
		return m
	}
	next, _ := m.Update(msg)
	return next
}

// press sends one key the way the terminal would. Named keys carry their own
// Code, the way repo_test.go's and detail_test.go's key() do; anything else
// is its own Code and Text.
func press(m Model, key string) (Model, tea.Cmd) {
	switch key {
	case "enter":
		return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "esc":
		return m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	case "space":
		return m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	case "backspace":
		return m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	default:
		return m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
	}
}

func TestTheFirstSearchAsksForWhatTheFormMeans(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	resolve(t, m, m.Init())

	if src.query != "is:open" {
		t.Errorf("searched for %q, want %q", src.query, "is:open")
	}
}

func TestTheResultsArriveOnTheModel(t *testing.T) {
	t.Parallel()

	src := &fakeSource{items: []gh.WorkItem{{Title: "fix the thing"}}}
	m := New(src)
	m = resolve(t, m, m.Init())

	if len(m.items) != 1 || m.items[0].Title != "fix the thing" {
		t.Errorf("items = %v, want the one the source returned", m.items)
	}
	if m.loading {
		t.Error("still loading after the answer arrived")
	}
}

// A query GitHub rejects is the user's to fix, so what it said has to reach
// the screen -- and the tab must keep working.
func TestARejectedQueryIsReportedOnTheTab(t *testing.T) {
	t.Parallel()

	src := &fakeSource{err: errors.New("gh api: Invalid search query")}
	m := New(src)
	m = resolve(t, m, m.Init())

	if m.notice == "" {
		t.Fatal("nothing to show the user about a rejected query")
	}
	if !strings.Contains(m.notice, "Invalid search query") {
		t.Errorf("notice = %q, want what GitHub said", m.notice)
	}
	if m.loading {
		t.Error("still loading after the failure arrived")
	}
}

// gh missing or a signed-out user is not something the tab can carry on
// past: the root shows its own screen for those.
func TestAFatalFailureGoesToTheRoot(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{})
	_, cmd := m.Update(errMsg{gen: m.gen, err: gh.ErrGhNotFound})
	if cmd == nil {
		t.Fatal("no message went to the root")
	}
	if _, ok := cmd().(FatalMsg); !ok {
		t.Errorf("sent %T, want FatalMsg", cmd())
	}
}

// An answer to a search the user has already typed past must not land.
func TestAnOldAnswerIsDropped(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m.gen = 2
	m.items = []gh.WorkItem{{Title: "current"}}
	next, _ := m.Update(itemsMsg{gen: 1, items: []gh.WorkItem{{Title: "stale"}}})
	if next.items[0].Title != "current" {
		t.Errorf("items = %v, want the stale answer dropped", next.items)
	}
}

func TestSpaceChangesAFilterWithoutSearching(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init()) // the first search
	src.query = ""

	m, cmd := press(m, " ")
	if m.filters.Value(FilterType) == "all" {
		t.Error("space did not move the filter")
	}
	if cmd != nil {
		t.Error("space started a search; the user is still choosing")
	}
}

func TestEnterOnAFilterRunsTheSearchItBuilt(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init())

	m, _ = press(m, " ") // type: all -> pr
	m, cmd := press(m, "enter")
	_ = resolve(t, m, cmd)

	if src.query != "is:open is:pr" {
		t.Errorf("searched for %q, want the filters' query", src.query)
	}
}

// A typed filter takes a value the way the add dialog does: the field opens,
// what is typed lands in it, and enter commits.
func TestTypingIntoAFilterReachesTheQuery(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init())

	m, _ = press(m, "j") // type -> state
	m, _ = press(m, "j") // state -> org
	m, _ = press(m, "enter")
	if !m.Capturing() {
		t.Fatal("the field is not open, so the root would act on q and 1")
	}
	for _, r := range "kukv" {
		m, _ = press(m, string(r))
	}
	m, cmd := press(m, "enter")
	m = resolve(t, m, cmd)

	if src.query != "is:open org:kukv" {
		t.Errorf("searched for %q, want the typed org in it", src.query)
	}
	if m.Capturing() {
		t.Error("the field is still open after enter")
	}
}

// The keys the root acts on before the tabs see them must reach the field.
func TestTypingAQuitKeyIntoAFieldTypesIt(t *testing.T) {
	t.Parallel()

	m := New(&fakeSource{})
	m, _ = press(m, "e") // the raw query editor
	for _, r := range "q1q23" {
		m, _ = press(m, string(r))
	}
	if !strings.Contains(m.View(), "q1q23") {
		t.Errorf("the typed query is not on screen:\n%s", m.View())
	}
}

// e edits the query the filters built; the filters are not parsed back out
// of what the user writes.
func TestTheEditedQueryIsWhatIsSearchedFor(t *testing.T) {
	t.Parallel()

	src := &fakeSource{}
	m := New(src)
	m = resolve(t, m, m.Init())

	m, _ = press(m, "e")
	for _, r := range " draft:false" {
		m, _ = press(m, string(r))
	}
	m, cmd := press(m, "enter")
	_ = resolve(t, m, cmd)

	if src.query != "is:open draft:false" {
		t.Errorf("searched for %q, want what the user edited", src.query)
	}
}

func TestEnterOnAResultOpensIt(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, []gh.WorkItem{{
		Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 41},
	}})
	m, _ = press(m, "l")
	_, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter on a result did nothing")
	}
	msg, ok := cmd().(OpenDetailMsg)
	if !ok {
		t.Fatalf("sent %T, want OpenDetailMsg", cmd())
	}
	if msg.Ref.Number != 41 {
		t.Errorf("opened #%d, want #41", msg.Ref.Number)
	}
}

// s belongs to saving a query, which this slice does not have yet. Binding
// it to anything else now would have to be taken back.
func TestSDoesNothingYet(t *testing.T) {
	t.Parallel()

	m := sized(t, 120, []gh.WorkItem{{Ref: gh.ItemRef{Repo: "kukv/octoscope", Number: 1}}})
	m, _ = press(m, "l")
	before := m.View()
	m, cmd := press(m, "s")
	if cmd != nil || m.View() != before {
		t.Error("s did something; it is reserved for saving a query")
	}
}

// Under 100 columns the filter pane is not drawn, so a cursor left on it is
// a cursor nothing on screen answers to. j, k, space and enter must not
// silently land on an invisible pane.
func TestNarrowWidthKeepsTheCursorOnWhatIsDrawn(t *testing.T) {
	t.Parallel()

	m := sized(t, 80, []gh.WorkItem{{
		Ref: gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 1},
	}})
	_, cmd := press(m, "enter")
	if cmd == nil {
		t.Fatal("enter did nothing; the cursor is stuck on the folded filter pane")
	}
	if _, ok := cmd().(OpenDetailMsg); !ok {
		t.Errorf("sent %T, want OpenDetailMsg", cmd())
	}
}

// onFilter moves the filter pane's cursor onto id with j/k, the way the user
// would, discarding whatever a landing along the way started.
func onFilter(t *testing.T, m Model, id FilterID) Model {
	t.Helper()

	m, _ = onFilterCmd(t, m, id)
	return m
}

// onFilterCmd is onFilter, but also hands back the tea.Cmd the landing step
// on id produced (nil if id was already the cursor, or if that filter
// already had its candidates), so a caller can check whether a fetch was
// actually started.
func onFilterCmd(t *testing.T, m Model, id FilterID) (Model, tea.Cmd) {
	t.Helper()

	var cmd tea.Cmd
	for m.cursor != id {
		if m.cursor < id {
			m, cmd = press(m, "j")
		} else {
			m, cmd = press(m, "k")
		}
	}
	return m, cmd
}

// withRepo types the golden repository into the repo filter's field, the
// way the user would, and resolves the search that committing a filter
// always starts. Every test that needs a named repository needs this one:
// it is the repository the candidate fixtures below belong to.
func withRepo(t *testing.T, m Model) Model {
	t.Helper()

	m = onFilter(t, m, FilterRepo)
	m, _ = press(m, "enter")
	for _, r := range "kukv/octoscope" {
		m, _ = press(m, string(r))
	}
	m, cmd := press(m, "enter")
	return resolve(t, m, cmd)
}

// resolveCandidates runs the candidate fetch the cursor's current filter
// (FilterLabel or FilterAuthor) starts, and feeds its answer back into
// Update, the way a landing on that row does once repo: names a repository.
func resolveCandidates(t *testing.T, m Model) Model {
	t.Helper()

	_, cmd := m.maybeFetchCandidates()
	if cmd == nil {
		t.Fatal("no candidate fetch to resolve")
	}
	return resolve(t, m, cmd)
}

// GitHub has no cross-repository list of labels, so there is nothing to
// offer until the search names one repository.
func TestNoCandidatesUntilARepositoryIsNamed(t *testing.T) {
	t.Parallel()

	src := &fakeSource{labels: []gh.Label{{Name: "bug"}}}
	m := New(src)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, m, m.Init())
	m = onFilter(t, m, FilterLabel)

	if strings.Contains(m.View(), "bug") {
		t.Errorf("candidates were offered with no repo: in the query:\n%s", m.View())
	}
	if src.labelRepo != "" {
		t.Errorf("asked for the labels of %q", src.labelRepo)
	}
}

func TestTheLabelsOfTheNamedRepositoryAreOffered(t *testing.T) {
	t.Parallel()

	src := &fakeSource{labels: []gh.Label{{Name: "bug"}, {Name: "enhancement"}}}
	m := New(src)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, m, m.Init())
	m = withRepo(t, m)
	m = onFilter(t, m, FilterLabel)
	m = resolveCandidates(t, m)

	if !strings.Contains(m.View(), "bug") {
		t.Errorf("the repository's labels are not offered:\n%s", m.View())
	}
}

// The same repository is not asked for a second time: the first landing on
// FilterLabel must start a fetch, and a later landing on the same repository
// must not start another one.
func TestTheSameRepositoryIsNotAskedForTwice(t *testing.T) {
	t.Parallel()

	src := &fakeSource{labels: []gh.Label{{Name: "bug"}}}
	m := New(src)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, m, m.Init())
	m = withRepo(t, m)

	m, cmd := onFilterCmd(t, m, FilterLabel)
	if cmd == nil {
		t.Fatal("landing on label with a newly named repo did not start a fetch")
	}
	m = resolve(t, m, cmd)

	m = onFilter(t, m, FilterAuthor)
	if _, cmd = onFilterCmd(t, m, FilterLabel); cmd != nil {
		t.Error("asked for the same repository's labels again")
	}
}

// author: offers the repository's assignable users the same way label: does.
func TestTheAuthorsOfTheNamedRepositoryAreOffered(t *testing.T) {
	t.Parallel()

	// octocat is not part of the repo name on screen, so a match cannot be
	// the repo: value bleeding through instead of an actual chip.
	src := &fakeSource{users: []string{"octocat"}}
	m := New(src)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, m, m.Init())
	m = withRepo(t, m)
	m = onFilter(t, m, FilterAuthor)
	m = resolveCandidates(t, m)

	if !strings.Contains(m.View(), "octocat") {
		t.Errorf("the repository's authors are not offered:\n%s", m.View())
	}
}

// "repo: not set means no candidates" applies to what is drawn, not only to
// what is fetched: clearing repo: must hide labels fetched for the
// repository it used to name, even though they are still cached.
func TestClearingTheRepoHidesItsStaleCandidates(t *testing.T) {
	t.Parallel()

	src := &fakeSource{labels: []gh.Label{{Name: "bug"}}}
	m := New(src)
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = resolve(t, m, m.Init())
	m = withRepo(t, m)
	m, cmd := onFilterCmd(t, m, FilterLabel)
	m = resolve(t, m, cmd)
	if !strings.Contains(m.View(), "bug") {
		t.Fatal("candidates never showed up; nothing to test clearing against")
	}

	m = onFilter(t, m, FilterRepo)
	m, _ = press(m, "enter")
	for range "kukv/octoscope" {
		m, _ = press(m, "backspace")
	}
	m, cmd = press(m, "enter")
	m = resolve(t, m, cmd)

	m = onFilter(t, m, FilterLabel)
	if strings.Contains(m.View(), "bug") {
		t.Errorf("a stale chip from the since-cleared repo: is still shown:\n%s", m.View())
	}
}
