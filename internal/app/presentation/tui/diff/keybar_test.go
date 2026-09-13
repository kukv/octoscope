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

// TestTheSubmitFooterFitsAt80InBothLanguages guards the same terminal-width
// guarantee for the popup's own footer, which is a fixed string rather than
// a fit-aware bar: it must never overrun 80 columns either.
func TestTheSubmitFooterFitsAt80InBothLanguages(t *testing.T) {
	for _, lang := range goldenLanguages {
		t.Run(lang.name, func(t *testing.T) {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			m := loaded(t, 80, 30)
			m, _ = m.Update(reviewMsg{ref: m.ref, ctx: threadFixture()})
			m = press(m, "v")
			if got := ansi.StringWidth(ansi.Strip(m.keyBar())); got > 80 {
				t.Errorf("submit footer is %d columns wide at 80: %q", got, ansi.Strip(m.keyBar()))
			}
		})
	}
}

// TestTheDiscardFooterFitsAt80InBothLanguages is the discard confirmation's
// share of the same guarantee.
func TestTheDiscardFooterFitsAt80InBothLanguages(t *testing.T) {
	for _, lang := range goldenLanguages {
		t.Run(lang.name, func(t *testing.T) {
			i18n.SetLanguage(lang.tag)
			t.Cleanup(func() { i18n.SetLanguage(language.English) })
			m := loaded(t, 80, 30)
			m, _ = m.Update(reviewMsg{ref: m.ref, ctx: threadFixture()})
			m.review.PendingID = "PRR_9"
			m = press(m, "X")
			if got := ansi.StringWidth(ansi.Strip(m.keyBar())); got > 80 {
				t.Errorf("discard footer is %d columns wide at 80: %q", got, ansi.Strip(m.keyBar()))
			}
		})
	}
}
