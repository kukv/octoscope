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
	return v
}

func (m Model) activeTab() string {
	if m.tab == tabRepos {
		return m.repo.View()
	}
	return m.work.View()
}

// tabRow labels each tab with the key that reaches it. Without a target
// repository the Repos tab is not offered at all (spec 3.4).
func (m Model) tabRow() string {
	labels := []string{"1 " + i18n.T("tab.work")}
	if m.opts.HasRepo {
		labels = append(labels, "2 "+i18n.T("tab.repos"))
	}

	for i, label := range labels {
		if tabID(i) == m.tab {
			labels[i] = theme.ActiveTab().Render(label)
		} else {
			labels[i] = theme.Dim().Render(label)
		}
	}
	return strings.Join(labels, "  ")
}

// errorView shows the failure that stopped the run. The heading and the key
// hint are ours and are short in both languages; the message itself came from
// gh or GitHub and can be any length, so it is wrapped rather than cut short
// (.claude/rules/errors.md).
func (m Model) errorView() string {
	return theme.Title().Render(i18n.T("app.error_title")) + "\n\n" +
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
