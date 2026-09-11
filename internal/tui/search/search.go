package search

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// searcher runs one GitHub search. The query is built here and means
// nothing to the layer below, which only sends it.
type searcher interface {
	SearchItems(ctx context.Context, query string) ([]gh.WorkItem, error)
}

// webOpener shows an item in a browser.
type webOpener interface {
	OpenWeb(url string) error
}

// Source is what the Search tab needs from the GitHub layer.
type Source interface {
	searcher
	webOpener
}

// FatalMsg carries a failure the parent shows on its error screen. Only what
// the user has to act on travels this way; a rejected query stays here as a
// notice (see gh.IsFatal).
type FatalMsg struct{ Err error }

type itemsMsg struct {
	gen   int
	items []gh.WorkItem
}

type errMsg struct {
	gen int
	err error
}

type Model struct {
	src Source

	filters Filters
	items   []gh.WorkItem

	// gen counts the searches started. An answer that names an older one is
	// dropped, which is what a query the user has since changed means.
	gen     int
	loading bool

	// notice is what GitHub said about a query it would not run. The tab
	// keeps its filters and its last results; the user edits and tries again.
	notice string

	spin          spinner.Model
	width, height int

	// sel is the cursor into items, drawn by the result pane.
	sel int

	// fetchedAt is when items last arrived, for the rows' relative ages:
	// View must not read the clock itself (.claude/rules/tui.md).
	fetchedAt time.Time
}

func New(src Source) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{src: src, loading: true, spin: s}
}

func (m Model) Init() tea.Cmd {
	return runSearch(m.src, m.filters.Query(), m.gen)
}

func runSearch(src searcher, query string, gen int) tea.Cmd {
	return func() tea.Msg {
		items, err := src.SearchItems(context.Background(), query)
		if err != nil {
			return errMsg{gen: gen, err: err}
		}
		return itemsMsg{gen: gen, items: items}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// The first size arrives after Init, so the spinner's tick loop
		// starts here rather than being batched with the search there.
		return m, m.spin.Tick
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case itemsMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.items = msg.items
		m.loading = false
		m.fetchedAt = time.Now()
		if m.sel >= len(m.items) {
			m.sel = max(len(m.items)-1, 0)
		}
		return m, nil
	case errMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.loading = false
		if gh.IsFatal(msg.err) {
			return m, func() tea.Msg { return FatalMsg{Err: msg.err} }
		}
		m.notice = msg.err.Error()
		return m, nil
	}
	return m, nil
}
