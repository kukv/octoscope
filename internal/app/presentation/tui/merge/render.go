package merge

import (
	"strings"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

// boxWidth is the popup's own cap, the width internal/app/presentation/tui/review draws its
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
	switch {
	case m.loading:
		b.WriteString(i18n.T("merge.loading") + "\n\n")
	case m.answered():
		b.WriteString(m.body())
	}
	b.WriteString(m.keyBar())
	// While the merge is in flight there are no keys to offer, and the blank
	// line the key bar would have filled is not a row worth drawing.
	return theme.Popup().Width(m.boxWidth()).Render(strings.TrimRight(b.String(), "\n"))
}

// answered reports whether a fetch has come back. Without one there is
// nothing true to draw: an empty context reads as a repository with no merge
// method, no auto-merge and an answer GitHub is still working out, none of
// which was measured. The failure itself is shown by the holder at footer
// level.
func (m Model) answered() bool { return m.ctx.PullRequest != "" }

func (m Model) body() string {
	var b strings.Builder
	for i, method := range m.ctx.Methods {
		b.WriteString(icon.Radio(i == m.row) + " " + methodText(method) + "\n")
	}
	b.WriteString("\n" + m.autoLine() + "\n" + theme.Dim().Render(m.deleteBranchText()) + "\n\n")
	if reason := m.reason(); reason != "" {
		b.WriteString(reason + "\n")
	}
	if m.sending {
		b.WriteString(i18n.T("merge.sending") + "\n")
	}
	return b.String()
}

func methodText(method domain.MergeMethod) string {
	switch method {
	case domain.MergeCommit:
		return i18n.T("merge.method_commit")
	case domain.MergeRebase:
		return i18n.T("merge.method_rebase")
	default:
		return i18n.T("merge.method_squash")
	}
}

// autoLine is one of three things: a pull request already in the queue only
// offers to leave it, one that can join shows the box space toggles, and one
// that cannot says why.
func (m Model) autoLine() string {
	switch {
	case m.ctx.AutoMergeEnabled:
		return i18n.T("merge.auto_on")
	case m.ctx.CanAutoMerge():
		return m.checkbox() + " " + i18n.T("merge.auto")
	case !m.ctx.AutoMergeAllowed:
		return theme.Dim().Render(i18n.T("merge.auto_unavailable_repo"))
	case !m.ctx.ViewerCanEnableAutoMerge:
		return theme.Dim().Render(i18n.T("merge.auto_unavailable_permission"))
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
// nothing refuses it, what the review still wants. A block an admin can push
// past carries a second line saying so: the key bar alone names the key but
// not that it is a protection being broken.
func (m Model) reason() string {
	if text := blockText(m.ctx.Block); text != "" {
		line := theme.Error().Render(icon.Warning() + " " + text)
		if m.ctx.CanMergeAsAdmin() {
			line += "\n" + theme.Dim().Render("  "+i18n.T("merge.admin_offer"))
		}
		return line
	}
	switch m.ctx.Review {
	case domain.ReviewRequired:
		return theme.Review(domain.ReviewRequired, false).Render(icon.Warning() + " " + i18n.T("merge.review_required"))
	case domain.ReviewChangesRequested:
		return theme.Review(domain.ReviewChangesRequested, false).Render(icon.Warning() + " " + i18n.T("merge.review_changes"))
	default:
		return ""
	}
}

func blockText(b domain.MergeBlock) string {
	switch b {
	case domain.BlockDraft:
		return i18n.T("merge.block_draft")
	case domain.BlockConflicting:
		return i18n.T("merge.block_conflicting")
	case domain.BlockComputing:
		return i18n.T("merge.block_computing")
	case domain.BlockProtected:
		return i18n.T("merge.block_protected")
	case domain.BlockBehind:
		return i18n.T("merge.block_behind")
	case domain.BlockDirty:
		return i18n.T("merge.block_dirty")
	default:
		return ""
	}
}

func (m Model) keyBar() string {
	hints := m.hints()
	if len(hints) == 0 {
		return ""
	}
	return theme.Dim().Render(layout.FitKeyBar(hints, m.boxWidth()-4))
}

// hints names only the keys that act in the state the popup is in. A bar
// that offers enter while merging is blocked, or space where the box is not
// on offer, teaches the wrong keys and makes the popup look broken when they
// do nothing.
func (m Model) hints() []string {
	switch {
	case m.sending:
		return nil // every key is ignored until GitHub answers
	case m.loading:
		return []string{i18n.T("merge.key_refresh"), i18n.T("merge.key_cancel")}
	}
	var hints []string
	switch {
	case m.ctx.AutoMergeEnabled:
		hints = append(hints, i18n.T("merge.key_auto_off"))
	case !m.answered() || m.ctx.Block != domain.BlockNone:
		// enter sends nothing: there is no answer, or something refuses it.
		// a does, where the viewer may push past what refuses it.
		if m.ctx.CanMergeAsAdmin() {
			hints = append(hints, i18n.T("merge.key_admin"))
		}
	case m.auto:
		hints = append(hints, i18n.T("merge.key_queue"))
	default:
		hints = append(hints, i18n.T("merge.key_merge"))
	}
	if m.ctx.CanAutoMerge() && !m.ctx.AutoMergeEnabled {
		hints = append(hints, i18n.T("merge.key_auto"))
	}
	return append(hints, i18n.T("merge.key_refresh"), i18n.T("merge.key_cancel"))
}

// boxWidth is the popup's outer width, border included: never wider than
// boxWidth and never wider than the terminal leaves. The text inside is four
// columns narrower, which is what the border and the padding take.
func (m Model) boxWidth() int { return min(boxWidth, max(m.width-4, 0)) }
