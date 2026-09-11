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
}

func (f *fakeSource) SearchItems(_ context.Context, query string) ([]gh.WorkItem, error) {
	f.query = query
	return f.items, f.err
}

func (f *fakeSource) OpenWeb(string) error { return nil }

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
// of what the user writes (spec section 4.3).
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

// Under 100 columns the filter pane is not drawn (spec section 4.6), so a
// cursor left on it is a cursor nothing on screen answers to. j, k, space
// and enter must not silently land on an invisible pane.
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
