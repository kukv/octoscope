package detail

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/i18n"
)

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
		pr:        domain.PR{Number: 1, Title: overlongTitle, State: domain.StateOpen, Body: overlongBody},
		labels:    []domain.Label{{Name: overlongLabel, Color: "ff0000"}},
		reviewCtx: domain.ReviewContext{PullRequest: "PR_1"},
	}
	closed := &fakeSource{pr: domain.PR{Number: 2, Title: overlongTitle, State: domain.StateClosed}}

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
		pr:        domain.PR{Number: 1, Title: overlongTitle, State: domain.StateOpen},
		reviewErr: errors.New(overlongTitle),
	}
	reviewErrDetail := sized(loaded(reviewErrSrc, prRef()))
	_, reviewErrCmd := reviewErrDetail.Update(key("v"))
	reviewErrShown, _ := reviewErrDetail.Update(reviewErrCmd())

	picker := sized(openPicker(t, f, prRef(), "l"))
	failed, _ := picker.Update(pickErrorMsg{ref: prRef(), err: errors.New(overlongTitle)})

	// Toggle a label and press enter without resolving the resulting cmd, so
	// the picker is caught mid "applying" render rather than already settled.
	applying, _ := picker.Update(key("space"))
	applying, _ = applying.Update(key("enter"))

	return map[string]string{
		"loading":         sized(New(f, prRef())).View(),
		"detail":          detail.View(),
		"detail_closed":   sized(loaded(closed, domain.ItemRef{Kind: domain.ItemPR, Number: 2})).View(),
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

// TestTheNewMetaKeysResolve catches a key that was added to one catalog and
// not the other before it reaches a view.
func TestTheNewMetaKeysResolve(t *testing.T) {
	ids := []string{
		"detail.meta.repo", "detail.meta.author", "detail.meta.state",
		"detail.meta.review", "detail.meta.checks", "detail.meta.branch",
		"detail.meta.changes", "detail.meta.assignees", "detail.meta.labels",
		"detail.meta.updated",
		"detail.section.description", "detail.section.comments",
		"detail.no_description", "state.draft_suffix",
	}
	for _, lang := range []language.Tag{language.English, language.Japanese} {
		i18n.SetLanguage(lang)
		t.Cleanup(func() { i18n.SetLanguage(language.English) })
		for _, id := range ids {
			i18n.AssertNoUnresolvedIDs(t, i18n.T(id))
		}
	}
	i18n.AssertNoUnresolvedIDs(t, i18n.Tn("detail.section.comments", 2))
}

// TestTheViewSplitsInTwoWhenItCan covers the threshold: the rule between the
// panes is what tells the two layouts apart.
func TestTheViewSplitsInTwoWhenItCan(t *testing.T) {
	f := &fakeSource{pr: domain.PR{
		Number: 12, Title: "a pr", Author: domain.Author{Login: "kukv"},
		State: domain.StateOpen, Body: "the description",
	}}

	for _, tc := range []struct {
		name  string
		width int
		split bool
	}{
		{"a wide terminal splits", 120, true},
		{"the threshold itself splits", 100, true},
		{"one column short does not", 99, false},
		{"eighty does not", 80, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := loaded(f, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12})
			m, _ = m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 24})

			got := strings.Contains(ansi.Strip(m.View()), "repository  kukv/octoscope #12")
			if got != tc.split {
				t.Errorf("split = %v at %d columns, want %v:\n%s",
					got, tc.width, tc.split, ansi.Strip(m.View()))
			}
		})
	}
}

// TestTheSingleColumnKeepsEveryFact is why the narrow layout joins the rows
// rather than dropping them.
func TestTheSingleColumnKeepsEveryFact(t *testing.T) {
	pr := domain.PR{
		Number: 12, Title: "a pr", Author: domain.Author{Login: "kukv"},
		State: domain.StateOpen, Review: domain.ReviewApproved, Body: "the description",
		Assignees: []domain.Author{{Login: "alice"}},
		Checks:    domain.Checks{Total: 2, Passed: 1, Failed: 1},
		Head:      "feat/x", Base: "main", Additions: 218, Deletions: 31,
	}
	m := loaded(&fakeSource{pr: pr}, domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	view := ansi.Strip(m.View())
	for _, want := range []string{"kukv/octoscope #12", "@kukv", "feat/x → main", "+218", "−31", "@alice"} {
		if !strings.Contains(view, want) {
			t.Errorf("the single-column header dropped %q:\n%s", want, view)
		}
	}
}

// TestNoLineOverrunsTheTerminal is the check a two-pane layout most easily
// fails, and the one a Japanese terminal fails first.
func TestNoLineOverrunsTheTerminal(t *testing.T) {
	for _, w := range []int{80, 100, 120, 160} {
		m := goldenModel(w)
		for _, l := range strings.Split(m.View(), "\n") {
			if got := ansi.StringWidth(l); got > w {
				t.Errorf("at %d columns a line is %d wide: %q", w, got, ansi.Strip(l))
			}
		}
	}
}

// TestTheKeyBarSurvivesAShortTerminal covers the one line the view cannot
// afford to lose: esc is the only way out of the detail view, and the key bar
// is what says so. A pull request has ten meta rows, which is taller than a
// short terminal's body, and JoinPanes runs to whichever pane is taller.
func TestTheKeyBarSurvivesAShortTerminal(t *testing.T) {
	for _, h := range []int{8, 12, 16, 24} {
		t.Run(fmt.Sprintf("height_%d", h), func(t *testing.T) {
			m := goldenModel(120)
			m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: h})

			lines := strings.Split(m.View(), "\n")
			if len(lines) > h {
				t.Errorf("the view is %d lines at a height of %d", len(lines), h)
			}
			last := ansi.Strip(lines[len(lines)-1])
			if !strings.Contains(last, "esc") {
				t.Errorf("the last line is %q, want the key bar", last)
			}
		})
	}
}

// TestAFailureSurvivesAShortTerminal covers the other line the cut must not
// swallow. What gh or GitHub said is the whole of what the reader has to go
// on after a failed close, so it belongs on the same side of the cut as the
// key bar (.claude/rules/errors.md); the meta pane's tail is what gives way.
func TestAFailureSurvivesAShortTerminal(t *testing.T) {
	const boom = "HTTP 403: Resource not accessible by integration"

	for _, h := range []int{8, 12, 16, 24} {
		t.Run(fmt.Sprintf("height_%d", h), func(t *testing.T) {
			m := goldenModel(120)
			m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: h})
			m, _ = m.Update(stateErrorMsg{ref: goldenRef(), err: errors.New(boom)})

			view := m.View()
			if !strings.Contains(ansi.Strip(view), boom) {
				t.Errorf("the failure is off the screen at a height of %d:\n%s", h, ansi.Strip(view))
			}
			lines := strings.Split(view, "\n")
			if len(lines) > h {
				t.Errorf("the view is %d lines at a height of %d", len(lines), h)
			}
			last := ansi.Strip(lines[len(lines)-1])
			if !strings.Contains(last, "esc") {
				t.Errorf("the last line is %q, want the key bar", last)
			}
		})
	}
}
