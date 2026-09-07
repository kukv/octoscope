package merge

import (
	"errors"
	"fmt"
	"strings"
	"testing"

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

// sized is loaded at the width being recorded: loaded opens at 120, and the
// popup has to be measured at the width the recording claims.
func sized(t *testing.T, f *fakeSource, width int) Model {
	t.Helper()

	m := loaded(t, f)
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	return m
}

// mergeCleanModel is a pull request with nothing left to wait for, so
// auto-merge is not on offer even though the repository allows it, and the
// review has not been given yet.
func mergeCleanModel(t *testing.T, width int) Model {
	c := mergeable()
	c.State = gh.MergeStateClean
	c.Review = gh.ReviewRequired
	c.DeleteBranchOnMerge = true
	c.AutoMergeAllowed = true
	c.ViewerCanEnableAutoMerge = true
	return sized(t, &fakeSource{ctx: c}, width)
}

// mergeAutoModel has space pressed, so the auto-merge box is ticked.
func mergeAutoModel(t *testing.T, width int) Model {
	c := mergeable()
	c.DeleteBranchOnMerge = true
	c.AutoMergeAllowed = true
	c.ViewerCanEnableAutoMerge = true
	m, _ := press(sized(t, &fakeSource{ctx: c}, width), " ")
	return m
}

// mergeAutoOnModel is a pull request already in the queue: the popup offers
// the cancellation rather than a box to tick (D6).
func mergeAutoOnModel(t *testing.T, width int) Model {
	c := mergeable()
	c.AutoMergeAllowed = true
	c.ViewerCanEnableAutoMerge = true
	c.AutoMergeEnabled = true
	return sized(t, &fakeSource{ctx: c}, width)
}

// mergeBlockedModel conflicts with the base branch, and the repository has
// auto-merge turned off.
func mergeBlockedModel(t *testing.T, width int) Model {
	c := mergeable()
	c.Mergeable = gh.MergeableConflicting
	c.State = gh.MergeStateDirty
	return sized(t, &fakeSource{ctx: c}, width)
}

// mergeComputingModel is a pull request GitHub has not worked out an answer
// for yet, which is the longest of the reasons in both languages.
func mergeComputingModel(t *testing.T, width int) Model {
	c := mergeable()
	c.Mergeable = gh.MergeableUnknown
	c.State = gh.MergeStateUnknown
	return sized(t, &fakeSource{ctx: c}, width)
}

// mergeSendingModel has enter pressed and the merge still in flight: the
// command it returned is deliberately left unrun.
func mergeSendingModel(t *testing.T, width int) Model {
	m, _ := enter(sized(t, &fakeSource{ctx: mergeable()}, width))
	return m
}

// mergeAutoForbiddenModel is a repository that allows auto-merge on a pull
// request this viewer may not queue.
func mergeAutoForbiddenModel(t *testing.T, width int) Model {
	c := mergeable()
	c.AutoMergeAllowed = true
	c.ViewerCanEnableAutoMerge = false
	return sized(t, &fakeSource{ctx: c}, width)
}

// mergeFailedModel is the popup whose fetch failed: it has no answer to draw,
// and the failure itself is shown by the holder under the box.
func mergeFailedModel(t *testing.T, width int) Model {
	t.Helper()

	m := New(&fakeSource{err: errors.New("gh: HTTP 500")}, ref())
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	m, _ = m.Update(m.Init()())
	return m
}

// mergeLoadingModel is the popup before its fetch has landed.
func mergeLoadingModel(t *testing.T, width int) Model {
	m := New(&fakeSource{ctx: mergeable()}, ref())
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
	return m
}

// goldenStates are the recorded states, by the name their recording carries.
var goldenStates = []struct {
	name  string
	build func(t *testing.T, width int) Model
}{
	{"merge_clean", mergeCleanModel},
	{"merge_auto", mergeAutoModel},
	{"merge_auto_on", mergeAutoOnModel},
	{"merge_blocked", mergeBlockedModel},
	{"merge_computing", mergeComputingModel},
	{"merge_auto_forbidden", mergeAutoForbiddenModel},
	{"merge_failed", mergeFailedModel},
	{"merge_sending", mergeSendingModel},
	{"merge_loading", mergeLoadingModel},
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				for _, state := range goldenStates {
					golden.Assert(t, fmt.Sprintf("%s_%s_%d", state.name, lang.name, w), state.build(t, w).View())
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
// mid-language-switch would race.
func TestNothingOverrunsTheTerminal(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			for _, state := range goldenStates {
				for i, line := range strings.Split(state.build(t, w).View(), "\n") {
					if got := ansi.StringWidth(line); got > w {
						t.Errorf("%s %s %d: line %d is %d columns:\n%s", state.name, lang.name, w, i, got, line)
					}
				}
			}
		}
	}
}

// TestTheKeyBarNeverDropsTheWayOut guards every golden state at every width:
// layout.FitKeyBar drops hints from the tail, so esc -- the only way out of
// the popup -- is the first to go when the wording grows. The key would go on
// working with nothing on screen to say so, and the goldens catch that only
// if someone reads the diff.
// It does not run in parallel with the rest of the package, for the reason
// TestNothingOverrunsTheTerminal gives.
func TestTheKeyBarNeverDropsTheWayOut(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			for _, state := range goldenStates {
				m := state.build(t, w)
				if len(m.hints()) == 0 {
					continue // a state that offers no key at all draws no bar
				}
				view := ansi.Strip(m.View())
				if want := i18n.T("merge.key_cancel"); !strings.Contains(view, want) {
					t.Errorf("%s %s %d: the key bar dropped %q:\n%s", state.name, lang.name, w, want, view)
				}
			}
		}
	}
}
