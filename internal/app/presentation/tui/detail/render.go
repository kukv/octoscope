package detail

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

const (
	// twoPaneMinColumns is where the view splits in two. It is the Work
	// board's own threshold for its drawer, which is the same shape of
	// problem: a hundred columns leave the meta pane thirty-three and the
	// body sixty-six.
	twoPaneMinColumns = 100

	// metaPaneMaxColumns caps the meta pane. Its values are short, and past
	// forty columns the width would be spent on nothing.
	metaPaneMaxColumns = 40
)

func twoPane(w int) bool { return w >= twoPaneMinColumns }

// metaPaneWidth is the left pane's share. The floor is the threshold: at a
// hundred columns this is already thirty-three.
func metaPaneWidth(w int) int { return min(w/3, metaPaneMaxColumns) }

func (m Model) View() string {
	if m.phase == phaseLoading {
		out := layout.ClipLines(m.spin.View()+" "+i18n.T("common.loading")+"\n", m.width)
		if m.declined != "" {
			out += layout.ClipLines(theme.Dim().Render(m.declined), m.width) + "\n"
		}
		return out
	}
	switch m.mode {
	case modeCompose:
		return m.composeView()
	case modeConfirm:
		return m.confirmView()
	case modeSubmit:
		return m.submitView()
	case modeMerge:
		return m.mergeView()
	case modePick:
		return m.pickerView()
	case modeView:
	}
	header := layout.ClipLines(theme.Title().Render(m.title), m.width)
	footer := layout.ClipLines(theme.Dim().Render(m.footer()), m.width)

	var b strings.Builder
	b.WriteString(header + "\n")
	if twoPane(m.width) {
		left := metaPaneLines(m.rows(), metaPaneWidth(m.width))
		right := strings.Split(m.body.View(), "\n")
		b.WriteString(strings.Join(layout.JoinPanes(left, right, metaPaneWidth(m.width)), "\n") + "\n")
	} else {
		for _, l := range m.headerLines() {
			b.WriteString(l + "\n")
		}
		// No rule of its own under the meta: the description's heading draws
		// one on the very next line, and two rules together say nothing the
		// first does not.
		b.WriteString(m.body.View() + "\n")
	}
	// The failure and the key bar are both drawn after the cut: what the
	// reader is told about a failure, and what tells them esc is the way out,
	// are the two things a short terminal must not swallow
	// (.claude/rules/errors.md).
	var failure string
	if m.errText != "" {
		failure = wrapErr(m.errText, m.width) + "\n"
	}
	height := m.height
	if height > 0 {
		// A wrapped failure can be taller than the terminal. Flooring the
		// budget at one keeps this a subtraction rather than a way back into
		// fitHeight's "no size yet" branch, where nothing would be cut at all.
		height = max(height-lineCount(failure), 1)
	}
	return fitHeight(b.String(), height) + failure + footer
}

// lineCount counts the lines in a block that ends in a newline, and returns
// zero for the empty string.
func lineCount(s string) int { return strings.Count(s, "\n") }

// fitHeight cuts what is drawn above the key bar down to the lines the
// terminal has, so that the key bar is still on the screen. The meta pane is
// as tall as the item has facts and does not scroll, and on a short terminal
// it would otherwise push the bar off the bottom -- taking with it the only
// notice that esc is the way out. What is lost instead is the tail of the
// pane, which the reader can get back by making the window taller.
//
// A height of zero or less means no size has arrived yet, and nothing is cut.
func fitHeight(s string, height int) string {
	if height <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	// The key bar is appended after this and takes the last line; s ends in a
	// newline, so its final element is the empty string before it.
	if budget := height - 1; len(lines) > budget {
		lines = append(lines[:budget], "")
	}
	return strings.Join(lines, "\n")
}

// rows is the meta pane's content. It is worked out at draw time rather than
// kept, because its layout follows the terminal's width.
func (m Model) rows() []metaRow {
	if !m.loaded {
		return nil
	}
	return metaRows(m.ref, m.item)
}

// headerLines is the meta block of the single-column layout: the same rows,
// joined into one wrapped paragraph above the body.
func (m Model) headerLines() []string {
	if !m.loaded {
		return nil
	}
	return metaInlineLines(m.rows(), m.width)
}

// footer builds the detail view's key bar from hints, most important first,
// dropping from the low-priority end when the terminal is too narrow for all
// of them -- the same mechanism diff's render.go uses for its own key bar.
func (m Model) footer() string {
	return layout.FitKeyBar(m.footerHints(), m.width)
}

// footerHints lists the detail view's hints, most important first. esc is
// first because layout.FitKeyBar never drops it: it is the only way out of
// the view. review, diff and checks only apply to a pull request; merge and
// state (close or reopen) only while the item is still open.
func (m Model) footerHints() []string {
	hints := []string{
		i18n.T("footer.detail.esc"),
		i18n.T("footer.detail.move"),
		i18n.T("footer.detail.comment"),
	}
	if m.ref.Kind == domain.ItemPR {
		hints = append(hints, i18n.T("footer.detail.review"), i18n.T("footer.detail.diff"),
			i18n.T("footer.detail.checks"))
	}
	if m.canMerge() {
		hints = append(hints, i18n.T("footer.detail.merge"))
	}
	if s := m.stateFooterKey(); s != "" {
		hints = append(hints, s)
	}
	return append(hints,
		i18n.T("footer.detail.refresh"),
		i18n.T("footer.detail.web"),
		i18n.T("footer.detail.labels"),
		i18n.T("footer.detail.assign"),
	)
}

// submitView draws the review popup: the title, the popup's own box, and a
// failed submission's error underneath it. The popup keeps no error text of
// its own, so what the user typed and chose is still there for a retry
// (.claude/rules/errors.md).
func (m Model) submitView() string {
	body := layout.ClipLines(theme.Title().Render(m.title), m.width) + "\n\n"
	body += m.submit.View() + "\n"
	if m.errText != "" {
		body += wrapErr(m.errText, m.width) + "\n"
	}
	return body + layout.ClipLines(theme.Dim().Render(i18n.T("footer.submit")), m.width)
}

// mergeView draws the merge popup with a failed merge's error underneath it.
// Unlike submitView it adds no key bar: the popup draws its own.
func (m Model) mergeView() string {
	body := layout.ClipLines(theme.Title().Render(m.title), m.width) + "\n\n"
	body += m.merge.View() + "\n"
	if m.errText != "" {
		body += wrapErr(m.errText, m.width) + "\n"
	}
	return body
}

// wrapErr lays out a failure that came from gh or GitHub. Unlike the hints and
// titles around it, this text is the whole of what the user has to go on, so
// it is wrapped rather than cut short (.claude/rules/errors.md).
func wrapErr(text string, w int) string {
	s := theme.Error().Render(i18n.T("common.error_prefix")) + text
	if w <= 0 {
		return s
	}
	return ansi.Wrap(s, w, "")
}

func (m Model) pickerView() string {
	body := m.picker.listView(m.height, m.width, m.errText)
	if m.phase == phaseWorking {
		return body + "\n" + layout.ClipLines(m.spin.View()+" "+i18n.T("picker.applying"), m.width) + "\n"
	}
	return body + "\n" + layout.ClipLines(theme.Dim().Render(i18n.T("footer.picker")), m.width)
}

// stateFooterKey returns the state-aware footer hint (with trailing spaces),
// or "" when the item cannot change state (merged / not yet loaded).
func (m Model) stateFooterKey() string {
	closing, ok := m.stateAction()
	if !ok {
		return ""
	}
	if closing {
		return i18n.T("footer.close")
	}
	return i18n.T("footer.reopen")
}

func (m Model) confirmView() string {
	header := theme.Title().Render(m.title)
	closing, _ := m.stateAction()
	var id string
	switch {
	case m.ref.Kind == domain.ItemPR && closing:
		id = "confirm.close_pr"
	case m.ref.Kind == domain.ItemPR:
		id = "confirm.reopen_pr"
	case closing:
		id = "confirm.close_issue"
	default:
		id = "confirm.reopen_issue"
	}
	var b strings.Builder
	b.WriteString(layout.ClipLines(header, m.width) + "\n\n")
	b.WriteString(i18n.T(id))
	if m.phase == phaseWorking {
		b.WriteString(m.spin.View() + " " + i18n.T("confirm.working") + "\n")
	} else {
		b.WriteString(theme.Dim().Render(i18n.T("confirm.yes_no")))
	}
	return layout.ClipLines(b.String(), m.width)
}

func (m Model) composeView() string {
	var b strings.Builder
	title := theme.Title().Render(i18n.Tf("compose.title", map[string]any{"Title": m.title}))
	b.WriteString(layout.ClipLines(title, m.width) + "\n\n")
	b.WriteString(m.textarea.View() + "\n\n")
	if m.errText != "" {
		b.WriteString(wrapErr(m.errText, m.width) + "\n\n")
	}
	if m.phase == phaseWorking {
		b.WriteString(layout.ClipLines(m.spin.View()+" "+i18n.T("compose.posting"), m.width) + "\n")
	} else {
		b.WriteString(layout.ClipLines(theme.Dim().Render(i18n.T("footer.compose")), m.width))
	}
	return b.String()
}

func cursorPrefix(selected bool) string {
	if selected {
		return "▸ "
	}
	return "  "
}
