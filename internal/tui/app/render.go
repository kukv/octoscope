package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/theme"
)

func (m Model) View() tea.View {
	var content string
	switch {
	case m.errText != "":
		content = m.errorView()
	case m.showingDetail:
		content = m.detail.View()
	default:
		content = m.tabRow() + "\n\n" + m.activeTab()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	// Nothing else turns the mouse on: without this the terminal reports no
	// clicks and no wheel at all (spec 4).
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m Model) activeTab() string {
	if m.tab == tabRepos {
		return m.repo.View()
	}
	return m.work.View()
}

// tabLabels names the tabs on offer, in display order. Without a target
// repository the Repos tab is not offered at all (spec 3.4). Both the tab row
// and the mouse hit-test read this, so they cannot disagree about where a
// label sits.
func (m Model) tabLabels() []string {
	labels := []string{"1 " + i18n.T("tab.work")}
	if m.opts.HasRepo {
		labels = append(labels, "2 "+i18n.T("tab.repos"))
	}
	return labels
}

// tabRow labels each tab with the key that reaches it, and reports on the
// board at the far right: what is waiting, what is broken, and how old the
// answer is. The summary lives here rather than on the board because it is
// true of the whole application, and is worth seeing from the Repos tab too.
func (m Model) tabRow() string {
	labels := m.tabLabels()
	for i, label := range labels {
		if tabID(i) == m.tab {
			labels[i] = theme.ActiveTab().Render(label)
		} else {
			labels[i] = theme.Dim().Render(label)
		}
	}
	row := strings.Join(labels, tabGap)
	if m.repoLookupTimedOut {
		row += tabGap + theme.Error().Render(i18n.T("tab.repo_lookup_timeout"))
	}

	summary := m.summary()
	pad := m.width - ansi.StringWidth(row) - ansi.StringWidth(summary)
	if summary == "" || pad < 2 {
		return row // too narrow to say anything beyond which tab this is
	}
	return row + strings.Repeat(" ", pad) + summary
}

// summary is the board's tally, in the order the mockup puts it. A count of
// zero is left out: the row is there to show what needs doing, and a row of
// zeroes is noise.
func (m Model) summary() string {
	s := m.work.Summary()
	if !s.Ready {
		return ""
	}
	var parts []string
	if s.Attention > 0 {
		parts = append(parts, theme.Count(true).Render(i18n.Tn("summary.attention", s.Attention)))
	}
	if s.Failing > 0 {
		parts = append(parts, theme.Error().Render(i18n.Tn("summary.failing", s.Failing)))
	}
	parts = append(parts, theme.Dim().Render(i18n.Tf("summary.updated", map[string]any{
		"Ago": i18n.RelTime(m.now, s.FetchedAt),
	})))
	return strings.Join(parts, theme.Dim().Render(" · "))
}

// errorView shows the failure that stopped the run. The heading and the key
// hint are ours and are short in both languages; the message itself came from
// gh or GitHub and can be any length, so it is wrapped rather than cut short
// (.claude/rules/errors.md).
func (m Model) errorView() string {
	return theme.Error().Bold(true).Render(i18n.T("app.error_title")) + "\n\n" +
		wrap(m.errText, m.width) + "\n\n" +
		theme.Dim().Render(i18n.T("footer.error"))
}

// wrap folds s to w display columns, breaking a word that has to be broken.
// A w of zero or less means there is no width yet.
func wrap(s string, w int) string {
	if w <= 0 {
		return s
	}
	return ansi.Wrap(s, w, "")
}
