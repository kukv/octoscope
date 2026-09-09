package app

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
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

// goldenModel is the root just after start-up. What the root draws itself is
// the tab row and the error screen; the board underneath is still fetching,
// and its loaded shapes are recorded by the board's own golden test.
func goldenModel(width int, opts Options) Model {
	m := New(&fakeSource{}, opts)
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	return next.(Model)
}

// goldenModelReady is goldenModel, but runs the fetch the WindowSizeMsg
// starts so the board's data is in and the tab row has a summary to draw.
func goldenModelReady(t *testing.T, width int, opts Options) Model {
	t.Helper()
	m := New(&fakeSource{}, opts)
	next, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
	return resolve(t, next.(Model), cmd)
}

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })

				// --repo starts on the Repos tab, so the board's own first
				// frame is reached with 1.
				withRepo := goldenModel(w, Options{Repo: "kukv/demo"})
				golden.Assert(t, fmt.Sprintf("app_tabs_%s_%d", lang.name, w), withRepo.View().Content)

				onWork := press(withRepo, "1")
				golden.Assert(t, fmt.Sprintf("app_tabs_work_%s_%d", lang.name, w), onWork.View().Content)

				failed := goldenModel(w, Options{})
				next, _ := failed.fail(errors.New("gh: HTTP 403: rate limit exceeded"))
				golden.Assert(t, fmt.Sprintf("app_error_%s_%d", lang.name, w), next.(Model).View().Content)

				missing := goldenModel(w, Options{})
				next, _ = missing.fail(gh.ErrGhNotFound)
				golden.Assert(t, fmt.Sprintf("app_gh_missing_%s_%d", lang.name, w), next.(Model).View().Content)

				badConfig := goldenModel(w, Options{Repo: "kukv/demo", ConfigError: "parse config.yaml: yaml: line 1: did not find expected node content"})
				golden.Assert(t, fmt.Sprintf("app_config_error_%s_%d", lang.name, w), badConfig.View().Content)

				// The tab row carries both the config warning and the
				// board's summary at once once loading finishes; a narrow
				// ja row must not lose the summary to make room for it.
				badConfigWithSummary := goldenModelReady(t, w, Options{Repo: "kukv/demo", ConfigError: "parse config.yaml: yaml: line 1: did not find expected node content"})
				golden.Assert(t, fmt.Sprintf("app_config_error_summary_%s_%d", lang.name, w), badConfigWithSummary.View().Content)
			})
		}
	}
}
