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
// way Bubble Tea would.
func resolve(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()

	if cmd == nil {
		t.Fatal("no command to run")
	}
	next, _ := m.Update(cmd())
	return next
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
