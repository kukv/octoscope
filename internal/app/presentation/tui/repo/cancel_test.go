package repo

import (
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// liveFetches runs the commands the way Bubble Tea would, but only after all
// of them have been handed out, and counts how many reached ListItems with a
// context still live. Holding them back is what stands in for requests still
// in flight while the cursor moves on.
func liveFetches(f *fakeSource, cmds []tea.Cmd) int {
	f.listCtxs = nil
	for _, cmd := range cmds {
		runAll(cmd)
	}
	live := 0
	for _, ctx := range f.listCtxs {
		if ctx.Err() == nil {
			live++
		}
	}
	return live
}

func runAll(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			runAll(c)
		}
	}
}

// Scrolling the sidebar passes over every row between here and there. Each
// one starts a fetch, and without cancelling the one before, all of them run
// to the end in parallel only for their answers to be dropped.
func TestMovingThroughTheSidebarLeavesOneFetchRunning(t *testing.T) {
	f := &fakeSource{}
	repos := []string{"a/1", "a/2", "a/3", "a/4", "a/5"}
	m := sized(New(f, Options{Repositories: repos}), 120)

	var cmds []tea.Cmd
	for i := range repos {
		var cmd tea.Cmd
		m, cmd = m.selectRow(i)
		cmds = append(cmds, cmd)
	}
	if got := liveFetches(f, cmds); got != 1 {
		t.Errorf("%d fetches ran with a live context, want 1: the row the cursor stopped on", got)
	}
}

// ranWithin runs cmd and reports whether it answered within d. A fetch
// answers at once against the fake; the wait for the cursor to rest does not.
func ranWithin(cmd tea.Cmd, d time.Duration) bool {
	if cmd == nil {
		return false
	}
	done := make(chan struct{})
	go func() {
		cmd()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// Cancelling a request GitHub has already received does not give its cost
// back, so stepping through the sidebar must not ask about the rows it only
// passes over. It asks once, for the row the cursor rests on.
func TestSteppingThroughTheSidebarAsksOnlyWhereTheCursorRests(t *testing.T) {
	f := &fakeSource{}
	repos := []string{"a/1", "a/2", "a/3", "a/4", "a/5"}
	m := sized(New(f, Options{Repositories: repos}), 120)
	m, _ = m.Update(key("h"))

	var stale int
	for range 4 {
		stale = m.gen
		var cmd tea.Cmd
		m, cmd = m.Update(key("j"))
		if ranWithin(cmd, 100*time.Millisecond) {
			t.Fatal("a step asked GitHub at once instead of waiting for the cursor to rest")
		}
	}
	if len(f.prRepos) != 0 {
		t.Fatalf("asked for %v while the cursor was still moving", f.prRepos)
	}

	if _, cmd := m.Update(rowSettledMsg{gen: stale}); cmd != nil {
		t.Error("a row the cursor has left was fetched once its wait ran out")
	}
	_, cmd := m.Update(rowSettledMsg{gen: m.gen})
	drain(t, cmd)
	if !slices.Equal(f.prRepos, []string{"a/5"}) {
		t.Errorf("asked for %v, want only the row the cursor rests on", f.prRepos)
	}
}

// Switching to Issues while the cursor settles must not ask straight away
// and then again when the wait ends; and r, which asks now, takes the wait's
// request over rather than being followed by it.
func TestTheWaitAsksOnceWhateverHappensDuringIt(t *testing.T) {
	f := &fakeSource{}
	m := sized(New(f, Options{Repositories: []string{"a/1", "a/2"}}), 120)
	m, _ = m.Update(key("h"))

	m, _ = m.Update(key("j"))
	m, cmd := m.Update(key("tab"))
	drain(t, cmd)
	m, cmd = m.Update(rowSettledMsg{gen: m.gen})
	drain(t, cmd)
	if len(f.prRepos) != 0 || !slices.Equal(f.issueRepos, []string{"a/2"}) {
		t.Errorf("PRs asked for %v, issues for %v; want issues for a/2, once", f.prRepos, f.issueRepos)
	}

	f.issueRepos = nil
	m, _ = m.Update(key("k"))
	m, cmd = m.Update(key("r"))
	for _, msg := range drain(t, cmd) {
		m, _ = m.Update(msg)
	}
	_, cmd = m.Update(rowSettledMsg{gen: m.gen})
	drain(t, cmd)
	if !slices.Equal(f.issueRepos, []string{"a/1"}) {
		t.Errorf("issues asked for %v, want a/1 once: r asked, and the wait must not ask again", f.issueRepos)
	}
}

// r pressed again before the list answers asks the same question again; the
// first asking is no longer wanted.
func TestRefreshingAgainLeavesOneFetchRunning(t *testing.T) {
	f := &fakeSource{}
	m := currentModel(f, 120)

	var cmds []tea.Cmd
	for range 3 {
		var cmd tea.Cmd
		m, cmd = m.Update(key("r"))
		cmds = append(cmds, cmd)
	}
	if got := liveFetches(f, cmds); got != 1 {
		t.Errorf("%d list fetches ran with a live context, want 1", got)
	}
}
