// Package repo lists the open pull requests and issues of one repository.
package repo

import (
	"context"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

// prSource is the pull-request half of what the list needs.
// The list always shows the client's own repository, so only the
// browser-opening call names one: repo is "owner/repo", and the empty string
// targets that same repository.
type prSource interface {
	ListPRs(ctx context.Context) ([]gh.PR, error)
	OpenPRWeb(repo string, number int) error
}

// issueSource mirrors prSource for issues.
type issueSource interface {
	ListIssues(ctx context.Context) ([]gh.Issue, error)
	OpenIssueWeb(repo string, number int) error
}

// repoNamer names the repository shown in the header. It stands alone
// because it belongs to neither kind.
type repoNamer interface {
	RepoName(ctx context.Context) (string, error)
}

// Source is what the repository list needs from the GitHub layer. A command
// that acts on one kind takes that half; the ones that pick the kind at run
// time take the whole.
type Source interface {
	prSource
	issueSource
	repoNamer
}

// OpenDetailMsg asks the parent to show the detail view for one item.
type OpenDetailMsg struct{ Ref gh.ItemRef }

// ErrorMsg carries a failure the parent shows on its error screen.
type ErrorMsg struct{ Err error }

type (
	prListMsg    []gh.PR
	issueListMsg []gh.Issue
	repoNameMsg  string
	errMsg       struct{ err error }
)

type tabID int

const (
	tabPRs tabID = iota
	tabIssues
)

type Model struct {
	src Source

	repoName string
	spin     spinner.Model
	width    int

	tab     tabID
	cursors [2]int
	prs     []gh.PR
	issues  []gh.Issue
	loaded  [2]bool
	loading [2]bool

	// fetchedAt is when the shown list arrived. The rows carry relative
	// times, and View must render the same string from the same state, so
	// the clock is read here in Update rather than on every draw.
	fetchedAt [2]time.Time
}

func New(src Source) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	m := Model{src: src, spin: s}
	m.loading[m.tab] = true
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, fetchRepoName(m.src), fetchList(m.src, m.tab))
}

func fetchList(src Source, t tabID) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		if t == tabPRs {
			prs, err := src.ListPRs(ctx)
			if err != nil {
				return errMsg{err}
			}
			return prListMsg(prs)
		}
		issues, err := src.ListIssues(ctx)
		if err != nil {
			return errMsg{err}
		}
		return issueListMsg(issues)
	}
}

func fetchRepoName(src repoNamer) tea.Cmd {
	return func() tea.Msg {
		name, err := src.RepoName(context.Background())
		if err != nil {
			return repoNameMsg("") // the name only decorates the header: a failure is not worth reporting
		}
		return repoNameMsg(name)
	}
}

func openWeb(src Source, ref gh.ItemRef) tea.Cmd {
	return func() tea.Msg {
		var err error
		if ref.Kind == gh.ItemPR {
			err = src.OpenPRWeb(ref.Repo, ref.Number)
		} else {
			err = src.OpenIssueWeb(ref.Repo, ref.Number)
		}
		if err != nil {
			return errMsg{err}
		}
		return nil
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	case repoNameMsg:
		m.repoName = string(msg)
		return m, nil
	case prListMsg:
		m.prs = []gh.PR(msg)
		m.loaded[tabPRs] = true
		m.fetchedAt[tabPRs] = time.Now()
		if m.cursors[tabPRs] >= len(m.prs) {
			m.cursors[tabPRs] = max(len(m.prs)-1, 0)
		}
		m.loading[tabPRs] = false
		return m, nil
	case issueListMsg:
		m.issues = []gh.Issue(msg)
		m.loaded[tabIssues] = true
		m.fetchedAt[tabIssues] = time.Now()
		if m.cursors[tabIssues] >= len(m.issues) {
			m.cursors[tabIssues] = max(len(m.issues)-1, 0)
		}
		m.loading[tabIssues] = false
		return m, nil
	case errMsg:
		m.loading[m.tab] = false
		err := msg.err
		return m, func() tea.Msg { return ErrorMsg{err} }
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		if m.tab == tabPRs {
			m.tab = tabIssues
		} else {
			m.tab = tabPRs
		}
		if !m.loaded[m.tab] {
			m.loading[m.tab] = true
			return m, fetchList(m.src, m.tab)
		}
		return m, nil
	case "j", "down":
		if n := m.itemCount(); n > 0 && m.cursors[m.tab] < n-1 {
			m.cursors[m.tab]++
		}
		return m, nil
	case "k", "up":
		if m.cursors[m.tab] > 0 {
			m.cursors[m.tab]--
		}
		return m, nil
	case "r":
		m.loading[m.tab] = true
		return m, fetchList(m.src, m.tab)
	case "enter":
		if ref, ok := m.SelectedRef(); ok {
			return m, func() tea.Msg { return OpenDetailMsg{ref} }
		}
		return m, nil
	case "o":
		if ref, ok := m.SelectedRef(); ok {
			return m, openWeb(m.src, ref)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) itemCount() int {
	if m.tab == tabPRs {
		return len(m.prs)
	}
	return len(m.issues)
}

// SelectedRef names the item under the cursor. ok is false when the tab is
// empty. Repo stays empty: the list only ever shows the client's repository.
func (m Model) SelectedRef() (gh.ItemRef, bool) {
	if m.tab == tabPRs {
		if len(m.prs) == 0 {
			return gh.ItemRef{}, false
		}
		return gh.ItemRef{Kind: gh.ItemPR, Number: m.prs[m.cursors[tabPRs]].Number}, true
	}
	if len(m.issues) == 0 {
		return gh.ItemRef{}, false
	}
	return gh.ItemRef{Kind: gh.ItemIssue, Number: m.issues[m.cursors[tabIssues]].Number}, true
}
