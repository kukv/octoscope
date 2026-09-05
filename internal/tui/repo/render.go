package repo

import (
	"fmt"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

func (m Model) View() string {
	var b strings.Builder
	title := i18n.T("app.name")
	if m.repoName != "" {
		title += " — " + m.repoName
	}
	b.WriteString(theme.Title().Render(title) + "\n\n")

	labels := subTabLabels()
	for i, label := range labels {
		if tabID(i) == m.tab {
			labels[i] = theme.ActiveTab().Render(label)
		} else {
			labels[i] = theme.Dim().Render(label)
		}
	}
	b.WriteString(strings.Join(labels, subTabGap) + "\n\n")

	switch {
	case m.loading[m.tab]:
		b.WriteString(m.spin.View() + " " + i18n.T("common.loading") + "\n")
	case m.tab == tabPRs && len(m.prs) == 0:
		b.WriteString(theme.Dim().Render(i18n.T("list.no_open_prs")) + "\n")
	case m.tab == tabIssues && len(m.issues) == 0:
		b.WriteString(theme.Dim().Render(i18n.T("list.no_open_issues")) + "\n")
	case m.tab == tabPRs:
		now := m.fetchedAt[tabPRs]
		for i, pr := range m.prs {
			b.WriteString(cursorPrefix(i == m.cursors[tabPRs]) + prLine(pr, now) + "\n")
		}
	default:
		now := m.fetchedAt[tabIssues]
		for i, issue := range m.issues {
			b.WriteString(cursorPrefix(i == m.cursors[tabIssues]) + issueLine(issue, now) + "\n")
		}
	}

	b.WriteString("\n" + theme.Dim().Render(i18n.T("footer.list")))
	return layout.ClipLines(b.String(), m.width)
}

// subTabLabels names the sub-tabs in display order. The hit-test walks the
// same list the row is drawn from, so a rename cannot move one without moving
// the other.
func subTabLabels() []string {
	return []string{i18n.T("list.tab_prs"), i18n.T("list.tab_issues")}
}

func cursorPrefix(selected bool) string {
	if selected {
		return "▸ "
	}
	return "  "
}

func prLine(pr gh.PR, now time.Time) string {
	review := gh.ParseReviewDecision(pr.ReviewDecision)
	return fmt.Sprintf("#%-5d %s  @%s  %s %s",
		pr.Number, pr.Title, pr.Author.Login,
		theme.Review(review, pr.IsDraft).Render(icon.Review(review, pr.IsDraft)),
		i18n.RelTime(now, pr.UpdatedAt))
}

func issueLine(issue gh.Issue, now time.Time) string {
	return fmt.Sprintf("#%-5d %s  @%s  %s",
		issue.Number, issue.Title, issue.Author.Login, i18n.RelTime(now, issue.UpdatedAt))
}
