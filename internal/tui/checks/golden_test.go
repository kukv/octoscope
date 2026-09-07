package checks

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/golden"
	"github.com/kukv/octoscope/internal/i18n"
)

// goldenWidths are the widths a screen most commonly opens at.
var goldenWidths = []int{160, 120, 80}

var goldenLanguages = []struct {
	name string
	tag  language.Tag
}{
	{"en", language.English},
	{"ja", language.Japanese},
}

// goldenLog is the failing check's log, one line Japanese: a recording is
// the only thing that catches column drift in the log pane, and all-ASCII
// source would hide it.
func goldenLog() []gh.LogLine {
	return []gh.LogLine{
		{Step: "Run tests", Time: time.Date(2026, 9, 7, 10, 15, 30, 0, time.UTC), Text: "FAIL ./internal/gh (TestWalk)"},
		{Text: "深さの上限に達したら探索を打ち切る"},
	}
}

// goldenModel is the list alone, before any log has been asked for. The
// cursor opens on row 0, which arrange puts on the failing check ("sca").
func goldenModel(width int) Model {
	m := New(&fakeSource{checks: fixture(), log: goldenLog()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: fixture()})
	return m
}

// checksLogModel is goldenModel with enter pressed, so the failing check's
// log -- goldenLog, Japanese line included -- is on screen.
func checksLogModel(width int) Model {
	m, cmd := goldenModel(width).Update(keyPress("enter"))
	m, _ = m.Update(cmd())
	return m
}

// checksRerunModel is goldenModel with the rerun popup open on the same
// failing check.
func checksRerunModel(width int) Model {
	return press(goldenModel(width), "R")
}

// checksLoadingModel is the view before its fetch has landed.
func checksLoadingModel(width int) Model {
	m := New(&fakeSource{checks: fixture(), log: goldenLog()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	return m
}

// mixed is one of each kind of check GitHub reports, in a list long enough to
// scroll: a failing workflow, a green one, a check run an App created (those
// come back with no workflow run behind them), and a StatusContext.
func mixed() gh.Checks {
	start := time.Date(2026, 9, 7, 10, 12, 0, 0, time.UTC)
	ran := func(r gh.CheckRun, took time.Duration) gh.CheckRun {
		r.Kind = gh.CheckKindRun
		r.URL = "https://github.example/job"
		r.StartedAt, r.CompletedAt = start, start.Add(took)
		return r
	}
	return gh.Checks{
		Total: 9, Passed: 7, Failed: 1, Running: 1, State: gh.CheckFailure,
		Runs: []gh.CheckRun{
			ran(gh.CheckRun{Name: "build", State: gh.CheckSuccess, Workflow: "CI", RunNumber: 88, JobID: 1, RunID: 10}, 3*time.Minute+7*time.Second),
			ran(gh.CheckRun{Name: "lint", State: gh.CheckSuccess, Workflow: "CI", RunNumber: 88, JobID: 2, RunID: 10}, 62*time.Second),
			ran(gh.CheckRun{Name: "test", State: gh.CheckSuccess, Workflow: "CI", RunNumber: 88, JobID: 3, RunID: 10}, 2*time.Minute+14*time.Second),
			ran(gh.CheckRun{Name: "sca", State: gh.CheckFailure, Workflow: "security", RunNumber: 116, JobID: 4, RunID: 20}, 48*time.Second),
			ran(gh.CheckRun{Name: "audit", State: gh.CheckSuccess, Workflow: "security", RunNumber: 116, JobID: 5, RunID: 20}, 31*time.Second),
			ran(gh.CheckRun{Name: "secrets", State: gh.CheckSuccess, Workflow: "security", RunNumber: 116, JobID: 6, RunID: 20}, 9*time.Second),
			ran(gh.CheckRun{Name: "deps", State: gh.CheckSuccess, Workflow: "security", RunNumber: 116, JobID: 7, RunID: 20}, 12*time.Second),
			// An App's own check run: no workflow, no run id, so no heading
			// over it and nothing for R to rerun.
			{
				Name: "codecov/patch", State: gh.CheckSuccess, Kind: gh.CheckKindRun,
				JobID: 8, URL: "https://codecov.example/1",
				StartedAt: start, CompletedAt: start.Add(6 * time.Second),
			},
			{Name: "ci/circleci", State: gh.CheckRunning, Kind: gh.CheckKindStatus, URL: "https://circleci.example/1"},
		},
	}
}

// checksMixedModel parks the cursor on the App-created check, far enough down
// a scrolled list that the first workflow's heading is off the top: j to the
// bottom, then one k, which leaves the window where it is.
func checksMixedModel(width int) Model {
	m := New(&fakeSource{checks: mixed()}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 12})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: mixed()})
	for range 8 {
		m = press(m, "j")
	}
	return press(m, "k")
}

// checksNoneModel is a pull request whose checks came back empty.
func checksNoneModel(width int) Model {
	m := New(&fakeSource{}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/octoscope", Number: 61})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(checksMsg{ref: m.ref, checks: gh.Checks{}})
	return m
}

// goldenStates are the recorded states, by the name their recording carries.
var goldenStates = []struct {
	name  string
	build func(width int) Model
}{
	{"checks", goldenModel},
	{"checks_log", checksLogModel},
	{"checks_rerun", checksRerunModel},
	{"checks_loading", checksLoadingModel},
	{"checks_none", checksNoneModel},
	{"checks_mixed", checksMixedModel},
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				for _, state := range goldenStates {
					golden.Assert(t, fmt.Sprintf("%s_%s_%d", state.name, lang.name, w), state.build(w).View())
				}
			})
		}
	}
}

// TestNothingOverrunsTheTerminal guards every golden state at every width:
// a row that overruns wraps, and everything under it draws a line lower
// than the layout believes.
// It does not run in parallel with the rest of the package: i18n.SetLanguage
// is global state, and a parallel test reading i18n.T while this one is
// mid-language-switch would race (internal/tui/diff's own golden test avoids
// the same way).
func TestNothingOverrunsTheTerminal(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			for _, state := range goldenStates {
				for i, line := range strings.Split(state.build(w).View(), "\n") {
					if got := ansi.StringWidth(line); got > w {
						t.Errorf("%s %s %d: line %d is %d columns:\n%s", state.name, lang.name, w, i, got, line)
					}
				}
			}
		}
	}
}
