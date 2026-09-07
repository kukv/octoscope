package merge

import (
	"strings"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/icon"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

// boxWidth is the popup's own cap, the width internal/tui/review draws its
// box at; boxWidth never exceeds what width actually leaves.
const boxWidth = 50

// View draws the popup's own box: the title, the methods the repository
// allows, what auto-merge and the branch will do, why merging is refused if
// it is, and the keys. The holder places this over what it already draws.
func (m Model) View() string {
	if m.width <= 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(theme.Title().Render(i18n.Tf("merge.title", map[string]any{"Number": m.ref.Number})) + "\n\n")
	if m.loading {
		b.WriteString(i18n.T("merge.loading") + "\n\n")
	} else {
		b.WriteString(m.body())
	}
	b.WriteString(m.keyBar())
	return theme.Popup().Width(m.boxWidth()).Render(b.String())
}

func (m Model) body() string {
	var b strings.Builder
	for i, method := range m.ctx.Methods {
		b.WriteString(icon.Radio(i == m.row) + " " + methodText(method) + "\n")
	}
	b.WriteString("\n" + m.autoLine() + "\n" + theme.Dim().Render(m.deleteBranchText()) + "\n\n")
	if reason := m.reason(); reason != "" {
		b.WriteString(reason + "\n")
	}
	return b.String()
}

func methodText(method gh.MergeMethod) string {
	switch method {
	case gh.MergeCommit:
		return i18n.T("merge.method_commit")
	case gh.MergeRebase:
		return i18n.T("merge.method_rebase")
	default:
		return i18n.T("merge.method_squash")
	}
}

// autoLine is one of three things: a pull request already in the queue only
// offers to leave it, one that can join shows the box space toggles, and one
// that cannot says why (standalone design §4.4.4, D6).
func (m Model) autoLine() string {
	switch {
	case m.ctx.AutoMergeEnabled:
		return i18n.T("merge.auto_on")
	case m.ctx.CanAutoMerge():
		return m.checkbox() + " " + i18n.T("merge.auto")
	case !m.ctx.AutoMergeAllowed:
		return theme.Dim().Render(i18n.T("merge.auto_unavailable_repo"))
	default:
		return theme.Dim().Render(i18n.T("merge.auto_unavailable_clean"))
	}
}

func (m Model) checkbox() string {
	if m.auto {
		return "[x]"
	}
	return "[ ]"
}

// deleteBranchText reads the repository's setting out. It carries no mark
// and the cursor never lands on it: GitHub's merge mutation takes no branch
// deletion input, so there is nothing here to press (D1).
func (m Model) deleteBranchText() string {
	if m.ctx.DeleteBranchOnMerge {
		return i18n.T("merge.delete_branch_on")
	}
	return i18n.T("merge.delete_branch_off")
}

// reason is the line under the options: why merging is refused, or, when
// nothing refuses it, what the review still wants.
func (m Model) reason() string {
	if text := blockText(m.ctx.Block()); text != "" {
		return theme.Error().Render(icon.Warning() + " " + text)
	}
	switch m.ctx.Review {
	case gh.ReviewRequired:
		return theme.Review(gh.ReviewRequired, false).Render(icon.Warning() + " " + i18n.T("merge.review_required"))
	case gh.ReviewChangesRequested:
		return theme.Review(gh.ReviewChangesRequested, false).Render(icon.Warning() + " " + i18n.T("merge.review_changes"))
	default:
		return ""
	}
}

func blockText(b gh.MergeBlock) string {
	switch b {
	case gh.BlockDraft:
		return i18n.T("merge.block_draft")
	case gh.BlockConflicting:
		return i18n.T("merge.block_conflicting")
	case gh.BlockComputing:
		return i18n.T("merge.block_computing")
	case gh.BlockProtected:
		return i18n.T("merge.block_protected")
	case gh.BlockBehind:
		return i18n.T("merge.block_behind")
	case gh.BlockDirty:
		return i18n.T("merge.block_dirty")
	default:
		return ""
	}
}

func (m Model) keyBar() string {
	hints := []string{
		i18n.T("merge.key_merge"),
		i18n.T("merge.key_auto"),
		i18n.T("merge.key_refresh"),
		i18n.T("merge.key_cancel"),
	}
	return theme.Dim().Render(layout.FitKeyBar(hints, m.boxWidth()-4))
}

// boxWidth is the popup's outer width, border included: never wider than
// boxWidth and never wider than the terminal leaves. The text inside is four
// columns narrower, which is what the border and the padding take.
func (m Model) boxWidth() int { return min(boxWidth, max(m.width-4, 0)) }
