package detail

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/app/presentation/tui/icon"
	"github.com/kukv/octoscope/internal/app/presentation/tui/layout"
	"github.com/kukv/octoscope/internal/app/presentation/tui/theme"
	"github.com/kukv/octoscope/internal/i18n"
)

// metaLabelWidth is the label column of the two-pane layout. layout.Pad cuts
// to one column short of what it is given, so this is the longest label
// ("リポジトリ", ten columns) plus two.
const metaLabelWidth = 12

// metaSep is what separates two facts on the same line, and metaSepWidth is
// what it costs: the middle dot is one column, not one byte.
const (
	metaSep      = " · "
	metaSepWidth = 3
)

// metaRow is one fact about the item. The value is already styled; the label
// is drawn by whichever layout is in use.
type metaRow struct {
	label string
	value string

	// inlineLabel keeps the label when the rows are joined into a single
	// paragraph. Only the assignees need it: "@alice" on its own reads as
	// the author.
	inlineLabel bool
}

// metaRows is everything the meta pane says about the item, in the order the
// design puts it. A row whose value is empty is left out rather than drawn
// with nothing after its label.
func metaRows(ref domain.ItemRef, it domain.Item) []metaRow {
	var rows []metaRow
	if ref.Repo != "" {
		rows = append(rows, metaRow{
			label: i18n.T("detail.meta.repo"),
			value: theme.Dim().Render(fmt.Sprintf("%s #%d", ref.Repo, it.Ref.Number)),
		})
	}
	rows = append(rows,
		metaRow{label: i18n.T("detail.meta.author"), value: "@" + it.Author.Login},
		metaRow{label: i18n.T("detail.meta.state"), value: itemStateText(it)},
	)
	if ch := it.Change; ch != nil {
		if ch.Review != domain.ReviewNone {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.review"),
				value: theme.Review(ch.Review, ch.IsDraft).Render(reviewText(ch.Review)),
			})
		}
		if ch.Checks.Total > 0 {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.checks"),
				value: checksText(ch.Checks),
			})
		}
		if ch.Head != "" && ch.Base != "" {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.branch"),
				value: theme.Accent().Render(ch.Head) +
					theme.Dim().Render(" → ") + theme.Accent().Render(ch.Base),
			})
		}
		if ch.Additions > 0 || ch.Deletions > 0 {
			rows = append(rows, metaRow{
				label: i18n.T("detail.meta.changes"),
				value: theme.Added().Render("+"+strconv.Itoa(ch.Additions)) + " " +
					theme.Removed().Render("−"+strconv.Itoa(ch.Deletions)),
			})
		}
	}
	if len(it.Assignees) > 0 {
		logins := make([]string, len(it.Assignees))
		for i, a := range it.Assignees {
			logins[i] = "@" + a.Login
		}
		rows = append(rows, metaRow{
			label:       i18n.T("detail.meta.assignees"),
			value:       strings.Join(logins, " "),
			inlineLabel: true,
		})
	}
	if len(it.Labels) > 0 {
		names := make([]string, len(it.Labels))
		for i, l := range it.Labels {
			names[i] = theme.Badge(l.Color).Render(" " + l.Name + " ")
		}
		rows = append(rows, metaRow{
			label: i18n.T("detail.meta.labels"),
			value: strings.Join(names, " "),
		})
	}
	return append(rows, metaRow{
		label: i18n.T("detail.meta.updated"),
		value: theme.Dim().Render(i18n.DateTime(it.UpdatedAt)),
	})
}

// itemStateText names the state, with the draft suffix a pull request can
// carry. A draft is open, and the state word alone would not say so.
func itemStateText(it domain.Item) string {
	s := stateText(it.State)
	if it.Change != nil && it.Change.IsDraft {
		s += i18n.T("state.draft_suffix")
	}
	return s
}

// checksText is the summary the detail view has room for: how many passed,
// failed and are still running. Which check is which is what the s key shows.
func checksText(c domain.Checks) string {
	var parts []string
	for _, p := range []struct {
		state domain.CheckState
		n     int
	}{
		{domain.CheckSuccess, c.Passed},
		{domain.CheckFailure, c.Failed},
		{domain.CheckRunning, c.Running},
	} {
		if p.n > 0 {
			parts = append(parts, theme.Check(p.state).Render(icon.Check(p.state)+strconv.Itoa(p.n)))
		}
	}
	return strings.Join(parts, " ")
}

// metaPaneLines draws the rows as the left pane: a dimmed label column and
// the value beside it, cut to whatever the pane was given.
func metaPaneLines(rows []metaRow, w int) []string {
	lines := make([]string, len(rows))
	for i, r := range rows {
		lines[i] = theme.Dim().Render(layout.Pad(r.label, metaLabelWidth)) +
			layout.Clip(r.value, max(w-metaLabelWidth, 0))
	}
	return lines
}

// metaInlineLines draws the same rows as one wrapped paragraph, for a
// terminal too narrow to give the pane a column of its own. Nothing is
// dropped: the paragraph grows by a line instead.
func metaInlineLines(rows []metaRow, w int) []string {
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = r.value
		if r.inlineLabel {
			parts[i] = theme.Dim().Render(r.label+" ") + r.value
		}
	}
	if len(parts) == 0 {
		return nil
	}
	sep := theme.Dim().Render(metaSep)
	if w <= 0 {
		return []string{strings.Join(parts, sep)}
	}

	// The paragraph is filled a fact at a time rather than wrapped a
	// character at a time: ansi.Wrap breaks on any space, including the
	// padding inside a label's badge, and a fact split across two lines
	// reads as two facts.
	var lines []string
	line := ""
	for _, p := range parts {
		p = layout.Clip(p, w) // a single fact wider than the terminal
		switch {
		case line == "":
			line = p
		case ansi.StringWidth(line)+metaSepWidth+ansi.StringWidth(p) <= w:
			line += sep + p
		default:
			lines = append(lines, line)
			line = p
		}
	}
	return append(lines, line)
}

// stateText and reviewText name a state in the reader's language. GitHub's
// own spelling stopped at the access layer (.claude/rules/architecture.md),
// and a state word is ours to translate.
func stateText(s domain.ItemState) string {
	switch s {
	case domain.StateOpen:
		return i18n.T("state.open")
	case domain.StateMerged:
		return i18n.T("state.merged")
	default:
		return i18n.T("state.closed")
	}
}

func reviewText(r domain.ReviewState) string {
	switch r {
	case domain.ReviewApproved:
		return i18n.T("review.approved")
	case domain.ReviewChangesRequested:
		return i18n.T("review.changes_requested")
	case domain.ReviewRequired:
		return i18n.T("review.required")
	default:
		return i18n.T("review.none")
	}
}
