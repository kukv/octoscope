// Package repo lists the open pull requests and issues of one repository.
package repo

import (
	"context"
	"errors"
	"slices"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/browser"
	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/tui/dialog"
)

// prSource is the pull-request half of what the list needs.
type prSource interface {
	ListPRs(ctx context.Context, repo string) ([]gh.PR, error)
}

// issueSource mirrors prSource for issues.
type issueSource interface {
	ListIssues(ctx context.Context, repo string) ([]gh.Issue, error)
}

// webOpener shows an item in a browser. It takes the URL GitHub gave the
// item rather than a reference to it: building the address by hand would put
// GitHub's URL layout in the UI.
type webOpener interface {
	OpenWeb(url string) error
}

// repoCounter is how many pull requests and issues are open in each
// repository of the sidebar. It stands alone because it is the only call
// that looks past the repository on screen.
type repoCounter interface {
	RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error)
}

// repoEditor is how the sidebar's own list changes: what to offer while the
// add dialog is typed into, what a first run can be seeded from, and where
// the result survives a restart.
type repoEditor interface {
	SearchRepos(ctx context.Context, query string, limit int) ([]gh.RepoCandidate, error)
	SeedCandidates(ctx context.Context) ([]gh.RepoCandidate, error)
	SaveRepositories(repos []string) error
}

// Source is what the repository list needs from the GitHub layer. A command
// that acts on one kind takes that half; the ones that pick the kind at run
// time take the whole.
type Source interface {
	prSource
	issueSource
	repoCounter
	webOpener
	repoEditor
}

// OpenDetailMsg asks the parent to show the detail view for one item.
type OpenDetailMsg struct{ Ref gh.ItemRef }

// OpenDiffMsg asks the parent to show the diff of the selected pull request.
type OpenDiffMsg struct{ Ref gh.ItemRef }

// OpenChecksMsg asks the parent to show the checks of the selected pull
// request.
type OpenChecksMsg struct{ Ref gh.ItemRef }

// FatalMsg carries a failure the parent shows on its error screen. Only what
// the user has to act on travels this way; everything else stays on the list
// as a notice (see gh.IsFatal).
type FatalMsg struct{ Err error }

type (
	prListMsg struct {
		gen int
		prs []gh.PR
	}
	issueListMsg struct {
		gen    int
		issues []gh.Issue
	}
	repoCountsMsg []gh.RepoCount

	// searchTickMsg fires once the typing has paused, and candidatesMsg
	// carries what the search it started found. Both name the generation of
	// the query they belong to, counted apart from the sidebar's own: moving
	// the cursor must not throw away suggestions being typed for.
	searchTickMsg struct{ gen int }
	candidatesMsg struct {
		gen        int
		candidates []gh.RepoCandidate
	}

	errMsg struct {
		gen  int
		tab  tabID
		kind noticeKind
		err  error
	}
)

// noticeKind says which failure the line above the key bar is reporting. The
// words in front of what the environment said differ: a browser that would
// not start has nothing to do with fetching, and saying it could not fetch
// would be a lie. The zero value is the fetch, which is where all but one of
// these come from.
type noticeKind uint8

const (
	noticeFetch noticeKind = iota
	noticeOpen
	noticeSave
)

// prefixID is the catalog key for the words in front of the notice. What the
// environment said follows them untranslated (.claude/rules/errors.md).
func (k noticeKind) prefixID() string {
	switch k {
	case noticeOpen:
		return "notice.open_failed"
	case noticeSave:
		return "notice.save_failed"
	default:
		return "notice.fetch_failed"
	}
}

// answeredFetch drops the notice of a tab whose fetch has just come back. It
// leaves an o's notice alone: no fetch answers a browser that would not
// start, and the address in that line is the only way the user has left to
// reach the item.
func (m *Model) answeredFetch(t tabID) {
	if m.notice[t].kind == noticeFetch {
		m.notice[t] = notice{}
	}
}

// noticeText is what the line carries after the words in front of it. A
// machine with no browser is octoscope's own finding, said in octoscope's own
// English, and the address is the whole of what the user can act on -- so
// that is what the line carries. Anything else was said by something outside
// octoscope and is shown as it was said (.claude/rules/errors.md).
func noticeText(err error) string {
	var noBrowser *browser.NoneError
	if errors.As(err, &noBrowser) {
		return noBrowser.URL
	}
	return err.Error()
}

// notice is what went wrong with the last fetch or the last o: the list
// carries on with what it already has, and r asks again. text is what the
// environment said, untranslated and so never empty while there is anything
// to report; the words around it are chosen when the line is drawn, because
// the language can change between two draws of the same model.
type notice struct {
	kind noticeKind
	text string
}

type tabID int

const (
	tabPRs tabID = iota
	tabIssues
)

// mode says which overlay is up. The list is what is drawn when none is.
type mode uint8

const (
	modeList mode = iota
	modeAdd
)

// pane says which half of the split screen the arrow keys move: the
// repository list on the left, or the table on the right.
type pane int

const (
	paneList pane = iota
	paneSidebar
)

// Options is what the list needs to know before it can draw: the
// repositories the settings file holds, and the one the user is standing in.
type Options struct {
	Repositories []string

	// Current is the repository --repo named, or the one the working
	// directory belongs to. It may be empty, and it may arrive after the
	// model was built: see SetCurrent.
	Current string
}

type Model struct {
	src  Source
	opts Options

	spin          spinner.Model
	width, height int

	rows     []row
	selected int
	focus    pane

	mode mode
	dlg  dialog.Model

	// searchGen counts the query the dialog is on. A tick or an answer from
	// an earlier one is dropped, which is what a query the user has since
	// typed past means.
	searchGen int

	tab     tabID
	cursors [2]int
	prs     []gh.PR
	issues  []gh.Issue
	loaded  [2]bool
	loading [2]bool

	// notice is the failure each tab carries on despite. It is per tab for the
	// same reason loaded and loading are: the two tabs are fetched separately,
	// so one answering says nothing about the other, and a single notice would
	// both vanish while the tab it described was still broken and follow the
	// user onto a tab that answered.
	notice [2]notice

	// gen counts how many times selectRow has run. A fetch or the errMsg it
	// can produce carries the generation it started in; Update drops one
	// whose generation no longer matches, which is what a row the cursor has
	// since left means. This does not depend on a repository's spelling, so
	// it survives the rename repoCountsMsg can give the selected row and
	// treats every fetch's failure the same way its success is treated.
	gen int

	// fetchedAt is when the shown list arrived. The rows carry relative
	// times, and View must render the same string from the same state, so
	// the clock is read here in Update rather than on every draw.
	fetchedAt [2]time.Time
}

func New(src Source, opts Options) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	m := Model{src: src, opts: opts}
	m.spin = s
	m.rows, m.selected = buildRows(opts.Repositories, opts.Current)
	if len(m.rows) > 0 {
		m.loading[m.tab] = true
	}
	return m
}

// SetCurrent names the repository the user is standing in once the lookup
// that finds it answers, which is seconds after the model was built. The
// answer can put a new row at the top of the list and select it, so it
// returns the fetch that row needs.
func (m Model) SetCurrent(name string) (Model, tea.Cmd) {
	m.opts.Current = name
	rows, selected := buildRows(m.opts.Repositories, name)
	m = m.setRows(rows)
	next, cmd := m.selectRow(selected)
	return next, tea.Batch(cmd, fetchCounts(next.src, next.rowNames()))
}

// Current is the repository the sidebar treats as the user's own, which the
// root asks for once the lookup that finds it answers.
func (m Model) Current() string { return m.opts.Current }

// selectRow is the single way the sidebar's cursor moves: from a key, from
// the mouse, and from the lookup that names the current repository. It
// clears the previous row's lists and starts fetching the new row's, which
// keeps the clear-and-refetch pair in the one place every path goes through.
//
// It does not skip the work when i already equals m.selected: SetCurrent can
// rebuild m.rows with a new temporary row at index 0 while the old selection
// was also 0, and an index alone cannot tell that row apart from the one it
// replaced.
func (m Model) selectRow(i int) (Model, tea.Cmd) {
	m.selected = i
	m.gen++
	m.prs, m.issues = nil, nil
	m.loaded, m.cursors = [2]bool{}, [2]int{}
	m.notice = [2]notice{}
	if len(m.rows) == 0 {
		return m, nil
	}
	m.loading[m.tab] = true
	return m, fetchList(m.src, m.tab, m.selectedRepo(), m.gen)
}

func (m Model) Init() tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	return tea.Batch(m.spin.Tick, fetchList(m.src, m.tab, m.selectedRepo(), m.gen), fetchCounts(m.src, m.rowNames()))
}

// Refresh re-fetches the current tab. The parent calls it after an event
// elsewhere changes what these lists show — a submitted review, say — the
// same way pressing r does.
func (m Model) Refresh() (Model, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, nil
	}
	m.loading[m.tab] = true
	// A tab that is being asked again has no failure to report until the new
	// request answers.
	m.answeredFetch(m.tab)
	return m, tea.Batch(fetchList(m.src, m.tab, m.selectedRepo(), m.gen), fetchCounts(m.src, m.rowNames()))
}

// rowNames is the sidebar's repositories in their current spelling, in the
// order fetchCounts must hand its answer back in.
func (m Model) rowNames() []string {
	names := make([]string, len(m.rows))
	for i, r := range m.rows {
		names[i] = r.name
	}
	return names
}

func fetchList(src Source, t tabID, repo string, gen int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if t == tabPRs {
			prs, err := src.ListPRs(ctx, repo)
			if err != nil {
				return errMsg{gen: gen, tab: t, err: err}
			}
			return prListMsg{gen: gen, prs: prs}
		}
		issues, err := src.ListIssues(ctx, repo)
		if err != nil {
			return errMsg{gen: gen, tab: t, err: err}
		}
		return issueListMsg{gen: gen, issues: issues}
	}
}

func fetchCounts(src repoCounter, repos []string) tea.Cmd {
	if len(repos) == 0 {
		return nil
	}
	return func() tea.Msg {
		counts, err := src.RepoCounts(context.Background(), repos)
		if err != nil {
			return repoCountsMsg(nil) // badges fall back to the dash; the lists still work
		}
		return repoCountsMsg(counts)
	}
}

func openWeb(src Source, url string, t tabID, gen int) tea.Cmd {
	return func() tea.Msg {
		if err := src.OpenWeb(url); err != nil {
			return errMsg{gen: gen, tab: t, kind: noticeOpen, err: err}
		}
		return nil
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.dlg = m.dlg.SetWidth(msg.Width)
		if m.sidebarCols() == 0 {
			m.focus = paneList
		}
		return m, nil
	case searchTickMsg:
		// The pause is over only for the query it was started for: every
		// keystroke schedules one, and all but the last are stale by now.
		if msg.gen != m.searchGen || m.mode != modeAdd {
			return m, nil
		}
		m.dlg = m.dlg.Searching()
		return m, m.runSearch(msg.gen)
	case candidatesMsg:
		if msg.gen != m.searchGen || m.mode != modeAdd {
			return m, nil
		}
		m.dlg = m.dlg.SetCandidates(msg.candidates)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case repoCountsMsg:
		if len(msg) != len(m.rows) {
			return m, nil
		}
		m.rows = slices.Clone(m.rows)
		for i, c := range msg {
			if c.Unavailable {
				continue
			}
			m.rows[i].name = c.Repo
			m.rows[i].prs, m.rows[i].issues = c.PRs, c.Issues
			m.rows[i].counted = true
		}
		return m, nil
	case prListMsg:
		// A fetch started for a row the cursor has since left must not land
		// under the row now selected.
		if msg.gen != m.gen {
			return m, nil
		}
		m.prs = msg.prs
		m.loaded[tabPRs] = true
		m.answeredFetch(tabPRs)
		m.fetchedAt[tabPRs] = time.Now()
		if m.cursors[tabPRs] >= len(m.prs) {
			m.cursors[tabPRs] = max(len(m.prs)-1, 0)
		}
		m.loading[tabPRs] = false
		return m, nil
	case issueListMsg:
		if msg.gen != m.gen {
			return m, nil
		}
		m.issues = msg.issues
		m.loaded[tabIssues] = true
		m.answeredFetch(tabIssues)
		m.fetchedAt[tabIssues] = time.Now()
		if m.cursors[tabIssues] >= len(m.issues) {
			m.cursors[tabIssues] = max(len(m.issues)-1, 0)
		}
		m.loading[tabIssues] = false
		return m, nil
	case errMsg:
		// Symmetric with prListMsg/issueListMsg above: a row the cursor has
		// left can still fail, and its failure must not touch the row now on
		// screen -- neither its loading spinner nor the full-screen error.
		// A save is not a fetch and belongs to no row, so it is never stale.
		if msg.kind != noticeSave && msg.gen != m.gen {
			return m, nil
		}
		// Only a fetch's failure ends a fetch. A browser that would not start
		// leaves whatever is in flight in flight.
		if msg.kind == noticeFetch {
			m.loading[msg.tab] = false
		}
		if gh.IsFatal(msg.err) {
			err := msg.err
			return m, func() tea.Msg { return FatalMsg{err} }
		}
		m.notice[msg.tab] = notice{kind: msg.kind, text: noticeText(msg.err)}
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseClickMsg:
		return m.handleMouseClick(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if m.mode == modeAdd {
		return m.handleAddKey(msg)
	}
	switch msg.String() {
	case "a":
		return m.openAddDialog()
	case "x":
		return m.removeSelected()
	case "tab":
		if m.tab == tabPRs {
			return m.showTab(tabIssues, true)
		}
		return m.showTab(tabPRs, true)
	case "h", "left":
		if m.sidebarCols() > 0 {
			m.focus = paneSidebar
		}
		return m, nil
	case "l", "right":
		m.focus = paneList
		return m, nil
	case "j", "down":
		if m.focus == paneSidebar {
			if m.selected < len(m.rows)-1 {
				return m.selectRow(m.selected + 1)
			}
			return m, nil
		}
		if n := m.itemCount(); n > 0 && m.cursors[m.tab] < n-1 {
			m.cursors[m.tab]++
		}
		return m, nil
	case "k", "up":
		if m.focus == paneSidebar {
			if m.selected > 0 {
				return m.selectRow(m.selected - 1)
			}
			return m, nil
		}
		if m.cursors[m.tab] > 0 {
			m.cursors[m.tab]--
		}
		return m, nil
	case "r":
		return m.Refresh()
	case "enter":
		if ref, ok := m.SelectedRef(); ok {
			return m, func() tea.Msg { return OpenDetailMsg{ref} }
		}
		return m, nil
	case "o":
		if url, ok := m.selectedURL(); ok {
			return m, openWeb(m.src, url, m.tab, m.gen)
		}
		return m, nil
	case "d":
		ref, ok := m.SelectedRef()
		// An issue has no diff. Opening an empty diff view would be a worse
		// answer than doing nothing.
		if !ok || ref.Kind != gh.ItemPR {
			return m, nil
		}
		return m, func() tea.Msg { return OpenDiffMsg{Ref: ref} }
	case "s":
		ref, ok := m.SelectedRef()
		// An issue has no checks.
		if !ok || ref.Kind != gh.ItemPR {
			return m, nil
		}
		return m, func() tea.Msg { return OpenChecksMsg{Ref: ref} }
	}
	return m, nil
}

func (m Model) itemCount() int {
	if m.tab == tabPRs {
		return len(m.prs)
	}
	return len(m.issues)
}

// selectedURL is the address GitHub gave the item under the cursor. ok is
// false when the tab is empty.
func (m Model) selectedURL() (string, bool) {
	if m.tab == tabPRs {
		if len(m.prs) == 0 {
			return "", false
		}
		return m.prs[m.cursors[tabPRs]].URL, true
	}
	if len(m.issues) == 0 {
		return "", false
	}
	return m.issues[m.cursors[tabIssues]].URL, true
}

// SelectedRef names the item under the cursor. ok is false when the tab is
// empty. The repository is the one this view is showing: the ref travels to
// the detail, diff and checks views, which draw it in their titles.
func (m Model) SelectedRef() (gh.ItemRef, bool) {
	if m.tab == tabPRs {
		if len(m.prs) == 0 {
			return gh.ItemRef{}, false
		}
		return gh.ItemRef{Kind: gh.ItemPR, Repo: m.selectedRepo(), Number: m.prs[m.cursors[tabPRs]].Number}, true
	}
	if len(m.issues) == 0 {
		return gh.ItemRef{}, false
	}
	return gh.ItemRef{Kind: gh.ItemIssue, Repo: m.selectedRepo(), Number: m.issues[m.cursors[tabIssues]].Number}, true
}
