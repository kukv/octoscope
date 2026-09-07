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
	md := issueMarkdown(issueItem(gh.Issue{Number: 3, Title: "an issue"}))
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
		pr:        gh.PR{Number: 1, Title: overlongTitle, State: gh.StateOpen, Body: overlongBody},
		labels:    []gh.Label{{Name: overlongLabel, Color: "ff0000"}},
		reviewCtx: gh.ReviewContext{PullRequestID: "PR_1"},
	}
	closed := &fakeSource{pr: gh.PR{Number: 2, Title: overlongTitle, State: gh.StateClosed}}

	sized := func(m Model) Model {
		m, _ = m.Update(size)
		return m
	}
	detail := sized(loaded(f, prRef()))
	compose, _ := detail.Update(key("c"))
	confirm, _ := detail.Update(key("x"))

	opening, cmd := detail.Update(key("v"))
	submit, _ := opening.Update(cmd())

	reviewErrSrc := &fakeSource{
		pr:        gh.PR{Number: 1, Title: overlongTitle, State: gh.StateOpen},
		reviewErr: errors.New(overlongTitle),
	}
	reviewErrDetail := sized(loaded(reviewErrSrc, prRef()))
	_, reviewErrCmd := reviewErrDetail.Update(key("v"))
	reviewErrShown, _ := reviewErrDetail.Update(reviewErrCmd())

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
		"submit":          submit.View(),
		"review_error":    reviewErrShown.View(),
		"picker":          picker.View(),
		"picker_applying": applying.View(),
		"picker_error":    failed.View(),
	}
}

// TestTheFooterNeverDropsEsc is detail's counterpart to diff's
// TestTheKeyBarNeverDropsEsc: esc is the only way out of the view, and it
// must survive whatever else the fit-aware footer drops at a narrow width.
func TestTheFooterNeverDropsEsc(t *testing.T) {
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		for _, width := range []int{50, 80, 100, 120} {
			screens := renderEveryScreenSized(t, width)
			// The open item draws the close footer and the closed one the
			// reopen footer; reopen is the wider of the two.
			for _, name := range []string{"detail", "detail_closed"} {
				esc := i18n.T("footer.detail.esc")
				if !strings.Contains(screens[name], esc) {
					t.Errorf("%s lang %s width %d: footer missing %q:\n%s",
						name, lang, width, esc, screens[name])
				}
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
