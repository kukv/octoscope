package search

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/app/domain"
)

type queryStore interface {
	SaveQueries(queries []domain.SavedQuery) error
}

// saveErrMsg carries a failure to write the saved queries to the settings
// file. Saving does not belong to any search generation the way itemsMsg
// and errMsg do, so it gets its own message type rather than reusing theirs
// (.claude/rules/errors.md).
type saveErrMsg struct{ err error }

// savedMsg says the saved queries reached the settings file.
type savedMsg struct{}

// SetSavedQueries loads the Search tab's saved queries, the way root.New
// hands the sidebar's list to the Repos tab.
func (m Model) SetSavedQueries(qs []domain.SavedQuery) Model {
	m.saved = qs
	return m
}

// upsert replaces the entry of the same name, or appends a new one. Two rows
// under one name is a list nobody can choose from.
func upsert(qs []domain.SavedQuery, q domain.SavedQuery) []domain.SavedQuery {
	for i, existing := range qs {
		if existing.Name == q.Name {
			out := slices.Clone(qs)
			out[i] = q
			return out
		}
	}
	return append(qs, q)
}

// removeSaved drops the entry at i and reports whether it was dropped, the
// way internal/app/presentation/tui/repo/rows.go's removeRow does for the sidebar's own x.
func removeSaved(qs []domain.SavedQuery, i int) ([]domain.SavedQuery, bool) {
	if i < 0 || i >= len(qs) {
		return qs, false
	}
	return append(slices.Clone(qs[:i]), qs[i+1:]...), true
}

// save writes the saved queries as they now stand, or, while a save is
// still out, marks them to be written once it answers. Two saves in flight
// together run in whichever order their goroutines happen to, and the older
// list written last would put a removed query back.
func (m Model) save() (Model, tea.Cmd) {
	if m.saving {
		m.saveAgain = true
		return m, nil
	}
	m.saving = true
	return m, saveQueries(m.src, m.saved)
}

// saveDone ends the save that was out, and writes the list as it now stands
// if it changed meanwhile -- after a failure too, since that list was never
// written.
func (m Model) saveDone() (Model, tea.Cmd) {
	m.saving = false
	if !m.saveAgain {
		return m, nil
	}
	m.saveAgain = false
	return m.save()
}

// saveQueries writes the saved queries to the settings file. It runs in a
// tea.Cmd so the write itself, and any failure, do not happen inside
// handleNameKey (.claude/rules/errors.md).
func saveQueries(src queryStore, qs []domain.SavedQuery) tea.Cmd {
	return func() tea.Msg {
		if err := src.SaveQueries(qs); err != nil {
			return saveErrMsg{err: err}
		}
		return savedMsg{}
	}
}
