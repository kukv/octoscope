package search

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/usecase"
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

// candidateSource fills the label and author chips under the filter pane:
// GitHub's own listings for one repository. There is no cross-repository
// listing, so these are only asked for once repo: names one.
type candidateSource interface {
	ListLabels(ctx context.Context, repo string) ([]gh.Label, error)
	ListAssignees(ctx context.Context, repo string) ([]string, error)
}

// Source is what the Search tab needs from the GitHub layer.
type Source interface {
	searcher
	webOpener
	candidateSource
	queryStore
}

// OpenDetailMsg asks the parent to show the detail view for one item.
type OpenDetailMsg struct{ Ref gh.ItemRef }

// OpenDiffMsg asks the parent to show the diff of the selected pull request.
type OpenDiffMsg struct{ Ref gh.ItemRef }

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

// webErrMsg carries a failure to open the browser. It is not tied to a
// search generation: the query the item came from may since have changed,
// but the browser call that failed has nothing to do with it.
type webErrMsg struct{ err error }

// labelCandidatesMsg and authorCandidatesMsg carry what a repository offers
// for the label and author chips. repo guards against a stale answer: the
// user may have typed a different repo: by the time it arrives.
type labelCandidatesMsg struct {
	repo   string
	labels []gh.Label
}

type authorCandidatesMsg struct {
	repo  string
	users []string
}

// pane says which half of the split screen j/k and enter act on.
type pane int

const (
	paneFilters pane = iota
	paneResults
)

// mode says which overlay is up: none, a typed filter's field, or the raw
// query editor. It does not stack with pane (.claude/rules/tui.md).
type mode int

const (
	modeBrowse mode = iota
	modeField
	modeRaw
	modeName
	modePicker
)

type Model struct {
	src Source

	filters Filters
	// raw is what the user typed into the raw query editor, and what a
	// search uses once it is set. Touching a filter clears it, since the
	// filters are not parsed back out of what was typed.
	raw string
	// cursor is the filter pane's own cursor, drawn as its selected row.
	cursor FilterID

	items []gh.WorkItem
	// sel is the cursor into items, drawn by the result pane.
	sel int

	pane  pane
	mode  mode
	input textinput.Model

	// gen counts the searches started. An answer that names an older one is
	// dropped, which is what a query the user has since changed means.
	gen     int
	loading bool

	// notice is what GitHub said about a query it would not run. The tab
	// keeps its filters and its last results; the user edits and tries again.
	notice string

	// saved is the Search tab's saved queries, in the order they were saved.
	saved []usecase.SavedQuery
	// pick is the picker's own cursor, drawn as its selected row.
	pick int

	// labelCandidates and authorCandidates are what the named repository
	// offers for the chips under the filter pane, kept with the repo they
	// were fetched for so the same repository is not asked for twice.
	labelCandidates      []gh.Label
	labelCandidatesRepo  string
	authorCandidates     []string
	authorCandidatesRepo string

	spin          spinner.Model
	width, height int

	// fetchedAt is when items last arrived, for the rows' relative ages:
	// View must not read the clock itself (.claude/rules/tui.md).
	fetchedAt time.Time
}

func New(src Source) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return Model{src: src, loading: true, spin: s}
}

// Capturing says every key belongs to the field or the raw editor while
// either is open. The root acts on q, 1, 2 and 3 before a tab sees them, and
// typing one of those into a query would quit octoscope or jump tabs
// mid-word.
func (m Model) Capturing() bool { return m.mode != modeBrowse }

// query is what a search runs for: the raw query once the user has edited
// one, otherwise what the filters mean.
func (m Model) query() string {
	if m.raw != "" {
		return m.raw
	}
	return m.filters.Query()
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, runSearch(m.src, m.query(), m.gen))
}

// Refresh re-runs the current search. The parent calls it after an event
// elsewhere changes what the results show — a submitted review or a merge,
// say — the same way pressing r does.
func (m Model) Refresh() (Model, tea.Cmd) {
	return m.startSearch()
}

// startSearch runs the query for the filters or raw text as they stand now.
func (m Model) startSearch() (Model, tea.Cmd) {
	m.gen++
	m.loading = true
	m.notice = ""
	return m, runSearch(m.src, m.query(), m.gen)
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

func openWeb(src webOpener, url string) tea.Cmd {
	return func() tea.Msg {
		if err := src.OpenWeb(url); err != nil {
			return webErrMsg{err: err}
		}
		return nil
	}
}

// fetchLabelCandidates and fetchAuthorCandidates ask for one repository's
// chips. A failure here is not shown: the chips are a convenience, and there
// is nothing the user has to act on, so it is dropped rather than taking the
// notice line (.claude/rules/errors.md).
func fetchLabelCandidates(src candidateSource, repo string) tea.Cmd {
	return func() tea.Msg {
		labels, err := src.ListLabels(context.Background(), repo)
		if err != nil {
			return nil
		}
		return labelCandidatesMsg{repo: repo, labels: labels}
	}
}

func fetchAuthorCandidates(src candidateSource, repo string) tea.Cmd {
	return func() tea.Msg {
		users, err := src.ListAssignees(context.Background(), repo)
		if err != nil {
			return nil
		}
		return authorCandidatesMsg{repo: repo, users: users}
	}
}

// maybeFetchCandidates asks for the chips of the filter pane's own cursor,
// once per repository. It is called on every cursor move rather than kept as
// a size-independent effect of the repo filter, since the chips are only
// worth having while the cursor sits on the row they belong to.
func (m Model) maybeFetchCandidates() (Model, tea.Cmd) {
	repo := m.filters.Value(FilterRepo)
	if repo == "" {
		return m, nil
	}
	switch m.cursor {
	case FilterLabel:
		if m.labelCandidatesRepo == repo {
			return m, nil
		}
		return m, fetchLabelCandidates(m.src, repo)
	case FilterAuthor:
		if m.authorCandidatesRepo == repo {
			return m, nil
		}
		return m, fetchAuthorCandidates(m.src, repo)
	}
	return m, nil
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if m.paneCols() == 0 {
			m.pane = paneResults
			// A typed filter's field is drawn inside the filter pane, which
			// just folded away; the raw editor draws its own row above the
			// panes regardless of width, so it is left open.
			if m.mode == modeField {
				m.mode = modeBrowse
			}
		}
		return m, nil
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
	case webErrMsg:
		m.notice = msg.err.Error()
		return m, nil
	case saveErrMsg:
		m.notice = i18n.T("search.save_failed") + ": " + msg.err.Error()
		return m, nil
	case labelCandidatesMsg:
		if msg.repo != m.filters.Value(FilterRepo) {
			return m, nil
		}
		m.labelCandidates = msg.labels
		m.labelCandidatesRepo = msg.repo
		return m, nil
	case authorCandidatesMsg:
		if msg.repo != m.filters.Value(FilterRepo) {
			return m, nil
		}
		m.authorCandidates = msg.users
		m.authorCandidatesRepo = msg.repo
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch m.mode {
	case modeField:
		return m.handleFieldKey(msg)
	case modeRaw:
		return m.handleRawKey(msg)
	case modeName:
		return m.handleNameKey(msg)
	case modePicker:
		return m.handlePickerKey(msg)
	}
	// ctrl+o is not a per-pane action: either pane opens the same list, so
	// it is handled here rather than inside handleFilterKey or
	// handleResultKey.
	if msg.String() == "ctrl+o" {
		return m.openPicker(), nil
	}
	if m.pane == paneFilters {
		return m.handleFilterKey(msg)
	}
	return m.handleResultKey(msg)
}

// openPicker opens the saved-queries list on its first row.
func (m Model) openPicker() Model {
	m.mode = modePicker
	m.pick = 0
	return m
}

func (m Model) handlePickerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		return m, nil
	case "j", "down":
		if m.pick < len(m.saved)-1 {
			m.pick++
		}
		return m, nil
	case "k", "up":
		if m.pick > 0 {
			m.pick--
		}
		return m, nil
	case "enter":
		if m.pick < 0 || m.pick >= len(m.saved) {
			return m, nil
		}
		m.raw = m.saved[m.pick].Query
		m.mode = modeBrowse
		return m.startSearch()
	}
	return m, nil
}

// handleFilterKey is browse mode with the filter pane focused.
func (m Model) handleFilterKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < filterCount-1 {
			m.cursor++
		}
		return m.maybeFetchCandidates()
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m.maybeFetchCandidates()
	case "space":
		m.filters = m.filters.Cycle(m.cursor)
		m.raw = ""
		return m, nil
	case "enter":
		if m.cursor.Choices() != nil {
			// space (above) clears raw when it moves a picked filter; enter
			// on the same row must mean the same thing, or committing a
			// filter here would silently run the untouched raw query
			// instead of what the filters now mean.
			m.raw = ""
			return m.startSearch()
		}
		return m.openField(), nil
	case "l", "right":
		if m.paneCols() > 0 {
			m.pane = paneResults
		}
		return m, nil
	case "e":
		return m.openRaw(), nil
	case "s":
		return m.openName(), nil
	case "r":
		return m.startSearch()
	}
	return m, nil
}

// handleResultKey is browse mode with the result pane focused.
func (m Model) handleResultKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.sel < len(m.items)-1 {
			m.sel++
		}
		return m, nil
	case "k", "up":
		if m.sel > 0 {
			m.sel--
		}
		return m, nil
	case "h", "left":
		if m.paneCols() > 0 {
			m.pane = paneFilters
		}
		return m, nil
	case "enter":
		ref, ok := m.selectedRef()
		if !ok {
			return m, nil
		}
		return m, func() tea.Msg { return OpenDetailMsg{Ref: ref} }
	case "d":
		ref, ok := m.selectedRef()
		// An issue has no diff. Opening an empty diff view would be a worse
		// answer than doing nothing.
		if !ok || ref.Kind != gh.ItemPR {
			return m, nil
		}
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
	case "o":
		if len(m.items) == 0 {
			return m, nil
		}
		return m, openWeb(m.src, m.items[m.sel].URL)
	case "e":
		return m.openRaw(), nil
	case "s":
		return m.openName(), nil
	case "r":
		return m.startSearch()
	}
	return m, nil
}

// selectedRef names the item under the result cursor. ok is false when
// there is nothing to select.
func (m Model) selectedRef() (gh.ItemRef, bool) {
	if m.sel < 0 || m.sel >= len(m.items) {
		return gh.ItemRef{}, false
	}
	return m.items[m.sel].Ref, true
}

// openField opens the input for the typed filter under the cursor, sized to
// the value column so a long name scrolls inside it rather than pushing the
// result pane's rule out of line.
func (m Model) openField() Model {
	m.mode = modeField
	m.input = textinput.New()
	m.input.SetValue(m.filters.Value(m.cursor))
	m.input.SetWidth(max(filterPaneWidth-filterNameWidth-promptCols, 1))
	m.input.Focus()
	return m
}

// openRaw opens the raw query editor on what the filters currently mean, or
// on what was last typed into it.
func (m Model) openRaw() Model {
	m.mode = modeRaw
	m.input = textinput.New()
	m.input.SetValue(m.query())
	if m.width > 0 {
		m.input.SetWidth(max(m.width-len("q ")-promptCols, 1))
	}
	m.input.Focus()
	return m
}

// openName opens the field that names a query before it is saved: saved
// queries hold a name, and a bare query string is not one a user can pick
// from later.
func (m Model) openName() Model {
	m.mode = modeName
	m.input = textinput.New()
	if m.width > 0 {
		m.input.SetWidth(max(m.width-ansi.StringWidth(i18n.T("search.save_prompt"))-promptCols, 1))
	}
	m.input.Focus()
	return m
}

func (m Model) handleFieldKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		return m, nil
	case "enter":
		m.filters = m.filters.Set(m.cursor, m.input.Value())
		m.raw = ""
		m.mode = modeBrowse
		return m.startSearch()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleRawKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		return m, nil
	case "enter":
		m.raw = m.input.Value()
		m.mode = modeBrowse
		return m.startSearch()
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) handleNameKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeBrowse
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.input.Value())
		m.mode = modeBrowse
		if name == "" {
			return m, nil
		}
		m.saved = upsert(m.saved, usecase.SavedQuery{Name: name, Query: m.query()})
		return m, saveQueries(m.src, m.saved)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
