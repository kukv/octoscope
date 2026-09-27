package repo

import (
	"testing"

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
