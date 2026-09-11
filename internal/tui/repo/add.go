package repo

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/dialog"
)

const (
	// searchDebounce is how long the dialog waits after the last keystroke
	// before it asks GitHub. One search has been measured at over a second,
	// so one per keystroke would queue a request behind every letter.
	searchDebounce = 400 * time.Millisecond

	// searchLimit is how many suggestions fit the box without scrolling it.
	searchLimit = 5
)

// openAddDialog puts up the popup that asks for a name.
func (m Model) openAddDialog() (Model, tea.Cmd) {
	m.mode = modeAdd
	m.dlg = dialog.New(i18n.T("dialog.add_repo_title"), i18n.T("dialog.add_repo_hint")).
		SetWidth(m.width)
	// Standing on the repository that leads the list without being in it, a
	// is almost always a request to keep that one, so it starts typed in.
	if i := m.selected; i < len(m.rows) && m.rows[i].temporary {
		m.dlg = m.dlg.SetValue(m.rows[i].name)
	}
	return m, nil
}

// seed opens the dialog on the repositories the user already has, which is
// the only way out of a first run's empty list that does not mean typing
// every name by hand. They are offered rather than added: an account can
// hold dozens, and adding all of them would leave x as the only way back.
func (m Model) seed() (Model, tea.Cmd) {
	m, _ = m.openAddDialog()
	m.dlg = m.dlg.Searching()
	m.searchGen++
	gen := m.searchGen
	src := m.src
	return m, func() tea.Msg {
		found, err := src.SeedCandidates(context.Background())
		if err != nil {
			return candidatesMsg{gen: gen}
		}
		return candidatesMsg{gen: gen, candidates: found}
	}
}

func (m Model) handleAddKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeList
		return m, nil
	case "enter":
		return m.addSelected()
	}
	before := m.dlg.Query()
	var cmd tea.Cmd
	m.dlg, cmd = m.dlg.Update(msg)
	if m.dlg.Query() == before {
		return m, cmd
	}
	m.searchGen++
	gen := m.searchGen
	return m, tea.Batch(cmd, tea.Tick(searchDebounce, func(time.Time) tea.Msg {
		return searchTickMsg{gen: gen}
	}))
}

// addSelected puts the dialog's answer in the list and writes the list out.
// The shape is checked here rather than on the way in: a suggestion is
// always well-formed, and rejecting a half-typed name would blink an error
// at someone who has not finished.
func (m Model) addSelected() (Model, tea.Cmd) {
	name := m.dlg.Value()
	if _, _, ok := gh.SplitRepo(name); !ok {
		m.dlg = m.dlg.SetError(i18n.T("dialog.invalid_name") + name)
		return m, nil
	}
	rows, added := addRow(m.rows, name)
	if !added {
		m.dlg = m.dlg.SetError(i18n.T("dialog.already_listed") + name)
		return m, nil
	}
	m.mode = modeList
	m = m.setRows(rows)
	next, cmd := m.selectRow(indexOf(rows, name))
	return next, tea.Batch(cmd,
		fetchCounts(next.src, next.rowNames()),
		saveRepos(next.src, savedNames(next.rows), next.tab))
}

// removeSelected drops the repository under the sidebar's cursor. It is
// gated on the focus: under a hundred columns the sidebar is folded away and
// h cannot reach it, so an ungated x would remove a row that is not drawn.
func (m Model) removeSelected() (Model, tea.Cmd) {
	if m.focus != paneSidebar {
		return m, nil
	}
	rows, removed := removeRow(m.rows, m.selected)
	if !removed {
		return m, nil
	}
	m = m.setRows(rows)
	// selectRow is the one path that clears the old row's lists and fetches
	// the new one's; going around it would leave the removed repository's
	// pull requests on screen.
	next, cmd := m.selectRow(min(m.selected, len(rows)-1))
	return next, tea.Batch(cmd,
		fetchCounts(next.src, next.rowNames()),
		saveRepos(next.src, savedNames(next.rows), next.tab))
}

// setRows keeps Options.Repositories in step with the rows on screen.
// SetCurrent rebuilds the rows from it when the repository lookup answers,
// which is up to twenty seconds after start-up: a list that is not kept here
// would lose everything added in the meantime.
func (m Model) setRows(rows []row) Model {
	m.rows = rows
	m.opts.Repositories = savedNames(rows)
	return m
}

func indexOf(rows []row, name string) int {
	for i, r := range rows {
		if r.name == name {
			return i
		}
	}
	return 0
}

func (m Model) runSearch(gen int) tea.Cmd {
	query := m.dlg.Query()
	if query == "" {
		return nil
	}
	src := m.src
	return func() tea.Msg {
		found, err := src.SearchRepos(context.Background(), query, searchLimit)
		if err != nil {
			return candidatesMsg{gen: gen}
		}
		return candidatesMsg{gen: gen, candidates: found}
	}
}

func saveRepos(src repoEditor, names []string, t tabID) tea.Cmd {
	return func() tea.Msg {
		if err := src.SaveRepositories(names); err != nil {
			return errMsg{tab: t, kind: noticeSave, err: err}
		}
		return nil
	}
}
