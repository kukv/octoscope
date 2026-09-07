package checks

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

// recordingSource embeds fakeSource and records what RerunWorkflow was
// called with, so a test can assert on the scope and run id it sent.
type recordingSource struct {
	fakeSource
	reruns int
	scope  gh.RerunScope
	runID  int64
}

func (s *recordingSource) RerunWorkflow(_ context.Context, _ string, runID int64, scope gh.RerunScope) error {
	s.reruns++
	s.scope = scope
	s.runID = runID
	return nil
}

func TestRerunAsksWhichScope(t *testing.T) {
	t.Parallel()

	m := press(open(t, 120), "R")
	if view := m.View(); !strings.Contains(view, i18n.T("checks.rerun_failed_only")) ||
		!strings.Contains(view, i18n.T("checks.rerun_all")) {
		t.Errorf("R did not offer both scopes:\n%s", view)
	}
}

func TestChoosingAScopeSendsIt(t *testing.T) {
	t.Parallel()

	src := &recordingSource{fakeSource: fakeSource{checks: fixture()}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m = press(m, "R")
	m = press(m, "j") // move from "only the failed jobs" to "the whole workflow"
	_, cmd := m.Update(keyPress("enter"))
	if cmd == nil {
		t.Fatal("enter sent nothing")
	}
	cmd()
	if src.scope != gh.RerunAll {
		t.Errorf("scope = %v, want RerunAll", src.scope)
	}
	// arrange puts the failing workflow first, so the cursor starts on sca,
	// whose run is 20.
	if src.runID != 20 {
		t.Errorf("runID = %d, want 20, the run of the selected check", src.runID)
	}
}

func TestEscLeavesTheRerunPopupWithoutSending(t *testing.T) {
	t.Parallel()

	src := &recordingSource{fakeSource: fakeSource{checks: fixture()}}
	m := New(src, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	m = press(press(m, "R"), "esc")
	if src.reruns != 0 {
		t.Errorf("esc sent %d reruns, want 0", src.reruns)
	}
	if view := m.View(); strings.Contains(view, i18n.T("checks.rerun_all")) {
		t.Errorf("the popup is still up after esc:\n%s", view)
	}
}

func TestRerunOnAStatusContextSaysWhyItCannot(t *testing.T) {
	t.Parallel()

	m := press(moveTo(t, open(t, 120), "ci/circleci"), "R")
	if view := m.View(); !strings.Contains(view, i18n.T("checks.decline_rerun_status_context")) {
		t.Errorf("R on a StatusContext said nothing:\n%s", view)
	}
}
