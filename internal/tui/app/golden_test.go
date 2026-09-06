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

func TestGolden(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(fmt.Sprintf("%s_%d", lang.name, w), func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })

				withRepo := goldenModel(w, Options{HasRepo: true})
				golden.Assert(t, fmt.Sprintf("app_tabs_%s_%d", lang.name, w), withRepo.View().Content)

				failed := goldenModel(w, Options{})
				next, _ := failed.fail(errors.New("gh: HTTP 403: rate limit exceeded"))
				golden.Assert(t, fmt.Sprintf("app_error_%s_%d", lang.name, w), next.(Model).View().Content)

				missing := goldenModel(w, Options{})
				next, _ = missing.fail(gh.ErrGhNotFound)
				golden.Assert(t, fmt.Sprintf("app_gh_missing_%s_%d", lang.name, w), next.(Model).View().Content)
			})
		}
	}
}
