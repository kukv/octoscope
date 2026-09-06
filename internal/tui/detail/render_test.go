package detail

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

func TestPRMarkdownContainsMetaBodyAndComments(t *testing.T) {
	pr := gh.PR{
		Number: 12, Title: "feat: pane", Author: gh.Author{Login: "kukv"},
		State: gh.StateOpen, IsDraft: true, Review: gh.ReviewRequired,
		Labels: []gh.Label{{Name: "Kind: Feature"}},
		Body:   "body text",
		Comments: []gh.Comment{
			{
				Author: gh.Author{Login: "bob"}, Body: "comment text",
				CreatedAt: time.Date(2026, 7, 11, 11, 0, 0, 0, time.UTC),
			},
		},
	}
	md := prMarkdown(pr)
	// The state and the review are named in the reader's language: GitHub's
	// own spelling stopped at the access layer.
	for _, want := range []string{
		"#12", "feat: pane", "@kukv",
		i18n.T("state.open") + i18n.T("md.draft_suffix"),
		i18n.T("review.required"), "Kind: Feature", "body text", "@bob", "comment text",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestIssueMarkdownEmptyBody(t *testing.T) {
	md := issueMarkdown(gh.Issue{Number: 3, Title: "an issue"})
	if !strings.Contains(md, "_no description_") {
		t.Errorf("markdown missing empty-body placeholder:\n%s", md)
	}
}

// overlongTitle is wider than any terminal the width test uses, in both
// scripts. Without it the fixture's longest line has room to spare at every
// width and the test would pass with the truncation removed.
const overlongTitle = "レンダリングのパイプラインをまるごと置き換える " +
	"refactor that nobody asked for"

// overlongBody carries an unbreakable token: glamour word-wraps prose, but a
// long URL has nowhere to break, so it is what proves the body stays inside
// the terminal.
const overlongBody = overlongTitle + "\n\nhttps://github.com/kukv/octoscope/pull/1#issuecomment-" +
	"0123456789012345678901234567890123456789012345678901234567890123456789\n"

const overlongLabel = "Kind: a label nobody would name this way — ラベル名が長すぎる場合"

// TestNoLineExceedsTheTerminalWidth guards spec §6.4 across every screen the
// detail view can show. A Japanese character occupies two columns, so a line
// that fits in English can still run off the screen in Japanese.
func TestNoLineExceedsTheTerminalWidth(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{50, 80, 100, 120} {
			for name, view := range renderEveryScreenSized(t, width) {
				for _, line := range strings.Split(view, "\n") {
					if w := ansi.StringWidth(line); w > width {
						t.Errorf("%s lang %s width %d: line is %d columns: %q",
							name, lang, width, w, line)
					}
				}
			}
		}
	}
}

// renderEveryScreenSized renders every screen at width, from a fixture whose
// title, label and error text all overflow. The closed item is here because
// only it draws the reopen footer, which is the widest of the two.
func renderEveryScreenSized(t *testing.T, width int) map[string]string {
	t.Helper()
	size := tea.WindowSizeMsg{Width: width, Height: 40}
	f := &fakeSource{
		pr:     gh.PR{Number: 1, Title: overlongTitle, State: gh.StateOpen, Body: overlongBody},
		labels: []gh.Label{{Name: overlongLabel, Color: "ff0000"}},
	}
	closed := &fakeSource{pr: gh.PR{Number: 2, Title: overlongTitle, State: gh.StateClosed}}

	sized := func(m Model) Model {
		m, _ = m.Update(size)
		return m
	}
	detail := sized(loaded(f, prRef()))
	compose, _ := detail.Update(key("c"))
	confirm, _ := detail.Update(key("x"))

	picker := sized(openPicker(t, f, prRef(), "l"))
	failed, _ := picker.Update(pickErrorMsg{err: errors.New(overlongTitle)})

	// Toggle a label and press enter without resolving the resulting cmd, so
	// the picker is caught mid "applying" render rather than already settled.
	applying, _ := picker.Update(key("space"))
	applying, _ = applying.Update(key("enter"))

	return map[string]string{
		"loading":         sized(New(f, prRef())).View(),
		"detail":          detail.View(),
		"detail_closed":   sized(loaded(closed, gh.ItemRef{Kind: gh.ItemPR, Number: 2})).View(),
		"compose":         compose.View(),
		"confirm":         confirm.View(),
		"picker":          picker.View(),
		"picker_applying": applying.View(),
		"picker_error":    failed.View(),
	}
}

// TestTheFooterIsWholeAtEightyColumns is the other half of the width guard.
// The width test alone is satisfied by truncation: a footer that grew to 81
// columns would be cut back to 80 and every assertion would stay green while
// esc:back vanished off the end. The suffix is the tail of the footer and so
// the first thing a cut takes, which is what makes it the thing to look for.
func TestTheFooterIsWholeAtEightyColumns(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		screens := renderEveryScreenSized(t, 80)
		// The open item draws the close footer and the closed one the reopen
		// footer; reopen is the wider of the two.
		for _, name := range []string{"detail", "detail_closed"} {
			suffix := i18n.T("footer.detail_suffix")
			if !strings.Contains(screens[name], suffix) {
				t.Errorf("%s lang %s: the footer was cut short of %q:\n%s",
					name, lang, suffix, screens[name])
			}
		}
	}
}

// TestNoUnresolvedIDsInRenderedViews guards spec §6.5. It renders each of the
// view's screens in both languages and fails when a message ID the code asked
// for is missing from that language's catalog. Walking i18n.IDs() cannot catch
// this: it only proves the catalog can resolve its own IDs, never that the IDs
// the code spells match them.
func TestNoUnresolvedIDsInRenderedViews(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for name, view := range renderEveryScreenSized(t, 0) {
			t.Run(lang.String()+"/"+name, func(t *testing.T) {
				i18n.AssertNoUnresolvedIDs(t, view)
			})
		}
	}
}
