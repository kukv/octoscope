package search

import (
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/usecase"
)

// queryStore is where the Search tab's saved queries survive a restart.
type queryStore interface {
	SaveQueries(queries []usecase.SavedQuery) error
}

// saveErrMsg carries a failure to write the saved queries to the settings
// file. Saving does not belong to any search generation the way itemsMsg
// and errMsg do, so it gets its own message type rather than reusing theirs
// (.claude/rules/errors.md).
type saveErrMsg struct{ err error }

// SetSavedQueries loads the Search tab's saved queries, the way app.New
// hands the sidebar's list to the Repos tab.
func (m Model) SetSavedQueries(qs []usecase.SavedQuery) Model {
	m.saved = qs
	return m
}

// upsert replaces the entry of the same name, or appends a new one. Two rows
// under one name is a list nobody can choose from.
func upsert(qs []usecase.SavedQuery, q usecase.SavedQuery) []usecase.SavedQuery {
	for i, existing := range qs {
		if existing.Name == q.Name {
			qs[i] = q
			return qs
		}
	}
	return append(qs, q)
}

// saveQueries writes the saved queries as they now stand to the settings
// file. It runs in a tea.Cmd so the write itself, and any failure, do not
// happen inside handleNameKey (.claude/rules/errors.md).
func saveQueries(src queryStore, qs []usecase.SavedQuery) tea.Cmd {
	return func() tea.Msg {
		if err := src.SaveQueries(qs); err != nil {
			return saveErrMsg{err: err}
		}
		return nil
	}
}
