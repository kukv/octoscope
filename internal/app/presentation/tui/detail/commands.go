// The commands in this file are what the detail view asks the outside world
// to do: fetch an item, post a comment, change a state, open a page. Each
// returns a tea.Cmd that runs off the update loop and reports back as a
// message, which is the only way a view is allowed to do I/O
// (.claude/rules/tui.md).

package detail

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
)

func fetch(src Source, ref domain.ItemRef) tea.Cmd {
	return func() tea.Msg {
		item, err := src.GetItem(context.Background(), ref)
		if err != nil {
			return errMsg{ref, err}
		}
		return itemMsg{ref, item}
	}
}

// fetchReviewContext is what v runs before it can open the review popup: an
// issue has no review, so this is only ever called on a pull request.
func fetchReviewContext(src reviewOpener, ref domain.ItemRef) tea.Cmd {
	return func() tea.Msg {
		ctx, err := src.PRReviewContext(context.Background(), ref.Repo, ref.Number)
		if err != nil {
			return reviewContextErrMsg{ref: ref, err: err}
		}
		return reviewContextMsg{ref: ref, ctx: ctx}
	}
}

func (m Model) openWeb(ref domain.ItemRef, url string) tea.Cmd {
	open := m.open
	return func() tea.Msg {
		if err := open(url); err != nil {
			return errMsg{ref, err}
		}
		return nil
	}
}

func postComment(src Source, ref domain.ItemRef, body string) tea.Cmd {
	return func() tea.Msg {
		if err := src.AddComment(context.Background(), ref, body); err != nil {
			return commentErrorMsg{ref: ref, err: err}
		}
		return commentPostedMsg{ref: ref}
	}
}

func setState(src Source, ref domain.ItemRef, closing bool) tea.Cmd {
	return func() tea.Msg {
		if err := src.SetState(context.Background(), ref, closing); err != nil {
			return stateErrorMsg{ref: ref, err: err}
		}
		return stateChangedMsg{ref: ref}
	}
}

func fetchLabelPicker(src candidateSource, ref domain.ItemRef) tea.Cmd {
	return func() tea.Msg {
		labels, err := src.ListLabels(context.Background(), ref.Repo)
		if err != nil {
			return pickErrorMsg{ref: ref, err: err}
		}
		return pickerCandidatesMsg{ref: ref, kind: pickLabels, labels: labels}
	}
}

func fetchAssigneePicker(src candidateSource, ref domain.ItemRef) tea.Cmd {
	return func() tea.Msg {
		users, err := src.ListAssignees(context.Background(), ref.Repo)
		if err != nil {
			return pickErrorMsg{ref: ref, err: err}
		}
		return pickerCandidatesMsg{ref: ref, kind: pickAssignees, users: users}
	}
}

func applyPicker(src Source, ref domain.ItemRef, kind pickerKind, add, remove []string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if kind == pickLabels {
			err = src.EditLabels(context.Background(), ref, add, remove)
		} else {
			err = src.EditAssignees(context.Background(), ref, add, remove)
		}
		if err != nil {
			return pickErrorMsg{ref: ref, err: err}
		}
		return pickerAppliedMsg{ref: ref}
	}
}
