package diff

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/i18n"
)

// TestTheKeyBarNeverDropsEsc is what would have caught the bug: adding
// c:comment pushed footer.diff past 80 columns, and what the naive
// truncation cut off was esc -- the only hint telling the user how to leave
// the view. esc must survive at every width the terminal is guaranteed to
// have.
func TestTheKeyBarNeverDropsEsc(t *testing.T) {
	for _, lang := range goldenLanguages {
		for _, w := range goldenWidths {
			t.Run(lang.name, func(t *testing.T) {
				i18n.SetLanguage(lang.tag)
				t.Cleanup(func() { i18n.SetLanguage(language.English) })
				m := loaded(t, w, 30)
				bar := ansi.Strip(m.keyBar())
				if !strings.Contains(bar, i18n.T("footer.diff.esc")) {
					t.Errorf("key bar at %d columns (%s) has no esc hint: %q", w, lang.name, bar)
				}
			})
		}
	}
}

// TestTheKeyBarFitsAt80InBothLanguages guards the terminal-width guarantee
// this project makes: whatever hints survive, the bar itself never overruns
// 80 columns.
func TestTheKeyBarFitsAt80InBothLanguages(t *testing.T) {
	for _, lang := range goldenLanguages {
		t.Run(lang.name, func(t *testing.T) {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			m := loaded(t, 80, 30)
			if got := ansi.StringWidth(ansi.Strip(m.keyBar())); got > 80 {
				t.Errorf("key bar is %d columns wide at 80: %q", got, ansi.Strip(m.keyBar()))
			}
		})
	}
}

// TestTheKeyBarShowsOnlyEscWhenNarrow is the other end of the fit-aware bar:
// when only one hint can fit, it is esc and nothing else -- no ellipsis, no
// partial hint.
func TestTheKeyBarShowsOnlyEscWhenNarrow(t *testing.T) {
	const narrow = 10 // fits "esc:back" but not "esc:back  c:comment"
	m := loaded(t, narrow, 30)
	bar := ansi.Strip(m.keyBar())
	if got := ansi.StringWidth(bar); got > narrow {
		t.Errorf("key bar is %d columns wide at %d: %q", got, narrow, bar)
	}
	if strings.TrimSpace(bar) != i18n.T("footer.diff.esc") {
		t.Errorf("key bar at %d columns = %q, want only the esc hint %q", narrow, bar, i18n.T("footer.diff.esc"))
	}
}

// TestTheKeyBarClipsEscWhenNarrowerThanTheHintItself covers fitKeyBar's last
// resort: below "esc:戻る"'s own 8 columns, even hints[:1] does not fit.
// esc must never be dropped, but it may be clipped -- the old fallback
// returned it unclipped and let the bar overrun the width budget.
func TestTheKeyBarClipsEscWhenNarrowerThanTheHintItself(t *testing.T) {
	const narrower = 5 // "esc:back" and "esc:戻る" are both wider than this
	for _, lang := range goldenLanguages {
		t.Run(lang.name, func(t *testing.T) {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			m := loaded(t, narrower, 30)
			bar := ansi.Strip(m.keyBar())
			if got := ansi.StringWidth(bar); got > narrower {
				t.Errorf("key bar is %d columns wide at %d: %q", got, narrower, bar)
			}
			if bar == "" {
				t.Error("esc was dropped instead of clipped")
			}
		})
	}
}
