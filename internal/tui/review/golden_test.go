package review

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/golden"
	"github.com/kukv/octoscope/internal/i18n"
)

var goldenWidths = []int{160, 120, 80}

var goldenLanguages = []struct {
	name string
	tag  language.Tag
}{
	{"en", language.English},
	{"ja", language.Japanese},
}

func goldenModel(width int) Model {
	m := New(&fakeSource{}, Target{PullRequestID: "PR_1", PendingID: "PRR_9", PendingComments: 2})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	m, _ = m.Update(keyPress("tab")) // approve, so a selected option is recorded too
	return m
}

// sendingModel is goldenModel after ctrl+s, with the submit cmd deliberately
// not run so the model is caught mid-send.
func sendingModel(width int) Model {
	m, _ := goldenModel(width).Update(keyPress("ctrl+s"))
	return m
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				golden.Assert(t, fmt.Sprintf("review_%s_%d", lang.name, w), goldenModel(w).View())
			})
		}
	}
}

// TestNoLineIsWiderThanTheTerminal guards the popup's own promise: it must
// fit inside the terminal it is drawn over, in both languages, at every width
// this project supports down to 80 columns.
func TestNoLineIsWiderThanTheTerminal(t *testing.T) {
	models := map[string]func(int) Model{
		"idle":    goldenModel,
		"sending": sendingModel,
	}
	for _, w := range goldenWidths {
		for _, lang := range goldenLanguages {
			for name, model := range models {
				t.Run(fmt.Sprintf("%s_%s_%d", name, lang.name, w), func(t *testing.T) {
					i18n.SetLanguage(lang.tag)
					t.Cleanup(func() { i18n.SetLanguage(language.English) })
					out := ansi.Strip(model(w).View())
					for i, line := range strings.Split(out, "\n") {
						if got := ansi.StringWidth(line); got > w {
							t.Errorf("line %d is %d columns wide in a terminal %d wide: %q", i, got, w, line)
						}
					}
				})
			}
		}
	}
}

// TestNoUnresolvedIDsInTheReviewView guards spec §6.5: a message ID the code
// asked for but the catalog does not have shows up literally, and this fails
// the moment it would be seen.
func TestNoUnresolvedIDsInTheReviewView(t *testing.T) {
	for _, lang := range goldenLanguages {
		t.Run(lang.name, func(t *testing.T) {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			noPending, _ := New(&fakeSource{}, Target{PullRequestID: "PR_1"}).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			for name, view := range map[string]string{
				"idle":       goldenModel(120).View(),
				"sending":    sendingModel(120).View(),
				"no_pending": noPending.View(),
			} {
				t.Run(name, func(t *testing.T) {
					i18n.AssertNoUnresolvedIDs(t, view)
				})
			}
		})
	}
}
