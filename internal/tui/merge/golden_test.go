package merge

import (
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
