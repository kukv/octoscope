package app

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/text/language"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/usecase"
)

// scenarioSource answers like the real thing does: a change lands, and the
// next fetch sees it. app_test.go's fakeSource returns fixed values, which
// cannot show a scenario reaching its end.
type scenarioSource struct {
	pr      gh.PR
	files   []gh.FileDiff
	labels  []gh.Label
	threads []gh.ReviewThread
	checks  gh.Checks

	pendingID string
	posted    []gh.PendingComment

	prRepos    []string
	issueRepos []string

	found []gh.RepoCandidate
	seed  []gh.RepoCandidate
	saved []string
}

func (f *scenarioSource) SearchRepos(context.Context, string, int) ([]gh.RepoCandidate, error) {
	return f.found, nil
}

func (f *scenarioSource) SeedCandidates(context.Context) ([]gh.RepoCandidate, error) {
	return f.seed, nil
}

func (f *scenarioSource) SaveRepositories(repos []string) error {
	f.saved = repos
	return nil
}

func (f *scenarioSource) ListWorkSection(_ context.Context, s gh.WorkSection) ([]gh.WorkItem, error) {
	if s != gh.SectionReviewRequested {
		return nil, nil
	}
	return []gh.WorkItem{{
		Ref:       gh.ItemRef{Kind: gh.ItemPR, Number: f.pr.Number},
		Title:     f.pr.Title,
		Author:    f.pr.Author.Login,
		UpdatedAt: f.pr.UpdatedAt,
	}}, nil
}

func (f *scenarioSource) ListPRs(_ context.Context, repo string) ([]gh.PR, error) {
	f.prRepos = append(f.prRepos, repo)
	return []gh.PR{f.pr}, nil
}

func (f *scenarioSource) ListIssues(_ context.Context, repo string) ([]gh.Issue, error) {
	f.issueRepos = append(f.issueRepos, repo)
	return nil, nil
}

func (f *scenarioSource) RepoName(context.Context) (string, error) { return "kukv/demo", nil }

func (f *scenarioSource) RepoCounts(context.Context, []string) ([]gh.RepoCount, error) {
	return nil, nil
}

func (f *scenarioSource) GetItem(context.Context, gh.ItemRef) (usecase.Item, error) {
	pr := f.pr
	return usecase.Item{
		Kind: gh.ItemPR, Number: pr.Number, Title: pr.Title, Author: pr.Author,
		State: pr.State, Body: pr.Body, URL: pr.URL, Labels: pr.Labels,
		Assignees: pr.Assignees, Comments: pr.Comments, UpdatedAt: pr.UpdatedAt,
		PR: &pr,
	}, nil
}

func (f *scenarioSource) OpenWeb(string) error { return nil }

func (f *scenarioSource) AddComment(_ gh.ItemRef, body string) error {
	f.pr.Comments = append(f.pr.Comments, gh.Comment{
		Author: gh.Author{Login: "kukv"}, Body: body, CreatedAt: scenarioAt,
	})
	return nil
}

func (f *scenarioSource) SetState(_ gh.ItemRef, closing bool) error {
	if closing {
		f.pr.State = gh.StateClosed
	} else {
		f.pr.State = gh.StateOpen
	}
	return nil
}

func (f *scenarioSource) EditLabels(_ gh.ItemRef, add, remove []string) error {
	for _, name := range add {
		f.pr.Labels = append(f.pr.Labels, gh.Label{Name: name})
	}
	for _, name := range remove {
		kept := f.pr.Labels[:0]
		for _, l := range f.pr.Labels {
			if l.Name != name {
				kept = append(kept, l)
			}
		}
		f.pr.Labels = kept
	}
	return nil
}

func (f *scenarioSource) EditAssignees(gh.ItemRef, []string, []string) error { return nil }

func (f *scenarioSource) ListLabels(context.Context, string) ([]gh.Label, error) {
	return f.labels, nil
}

func (f *scenarioSource) ListAssignees(context.Context, string) ([]string, error) { return nil, nil }

func (f *scenarioSource) PRDiff(context.Context, string, int) ([]gh.FileDiff, error) {
	return f.files, nil
}

func (f *scenarioSource) PRReviewContext(context.Context, string, int) (gh.ReviewContext, error) {
	return gh.ReviewContext{
		PullRequestID: "PR_1", PendingID: f.pendingID, Threads: f.threads,
	}, nil
}

// PostLineComment starts the pending review the first time, the way the
// usecase does.
func (f *scenarioSource) PostLineComment(_ usecase.ReviewTarget, c gh.PendingComment) (string, error) {
	f.posted = append(f.posted, c)
	f.pendingID = "PRR_1"
	f.threads = append(f.threads, gh.ReviewThread{
		Path: c.Path, Line: c.Line, Side: c.Side,
		Comments: []gh.ThreadComment{{Author: gh.Author{Login: "kukv"}, Body: c.Body, Pending: true}},
	})
	return f.pendingID, nil
}

func (f *scenarioSource) DiscardReview(string) error { return nil }

func (f *scenarioSource) SubmitReview(usecase.ReviewTarget, gh.ReviewEvent, string) error {
	return nil
}

func (f *scenarioSource) PRChecks(context.Context, string, int) (gh.Checks, error) {
	return f.checks, nil
}

func (f *scenarioSource) JobLog(context.Context, string, int64, bool) ([]gh.LogLine, error) {
	return nil, nil
}

func (f *scenarioSource) RerunWorkflow(context.Context, string, int64, gh.RerunScope) error {
	return nil
}

func (f *scenarioSource) PRMergeContext(context.Context, string, int) (gh.MergeContext, error) {
	return gh.MergeContext{}, nil
}

func (f *scenarioSource) MergePR(string, gh.MergeMethod) error         { return nil }
func (f *scenarioSource) EnableAutoMerge(string, gh.MergeMethod) error { return nil }
func (f *scenarioSource) DisableAutoMerge(string) error                { return nil }

var scenarioAt = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func scenarioPR() gh.PR {
	return gh.PR{
		Number: 12, Title: "replace the renderer",
		Author: gh.Author{Login: "kukv"}, State: gh.StateOpen,
		UpdatedAt: scenarioAt, Body: "body",
	}
}

// run presses each key and lets whatever it started finish, so a scenario
// reads as the keys a user types.
func run(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, cmd := pressCmd(m, k)
		m = resolve(t, next, cmd)
	}
	return m
}

func scenarioModel(t *testing.T, f *scenarioSource) Model {
	t.Helper()
	return scenarioModelWithRepos(t, f, nil)
}

// scenarioModelWithRepos starts the app with a settings file that already
// lists repositories, which is what the sidebar needs before x has anything
// to take away.
func scenarioModelWithRepos(t *testing.T, f *scenarioSource, repos []string) Model {
	t.Helper()
	i18n.SetLanguage(language.English)
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	// No --repo: the repository is the working directory's, so the app starts
	// on the board and the Repos tab appears when the lookup answers.
	m := New(f, Options{Repositories: repos})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return resolve(t, next.(Model), cmd)
}

func TestClosingFromTheReposTabShowsTheNewState(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR()}
	m := scenarioModel(t, f)

	m = run(t, m, "2")
	// The Work board's own enter opens the same detail view, so without
	// this the scenario would still pass with no Repos tab at all.
	if m.tab != tabRepos {
		t.Fatalf("precondition: tab = %d, want the Repos tab", m.tab)
	}

	m = run(t, m, "enter") // the Repos row -> the detail view
	// The Repos row carries the number too; the state line is detail's alone.
	if !strings.Contains(content(m), "state: open") {
		t.Fatalf("the detail view did not open:\n%s", content(m))
	}

	m = run(t, m, "x", "y")

	if f.pr.State != gh.StateClosed {
		t.Fatalf("state = %v, want closed", f.pr.State)
	}
	if got := content(m); !strings.Contains(got, "state: closed") {
		t.Errorf("the view does not show the new state:\n%s", got)
	}
}

func TestPickingALabelFromTheDetailViewAppliesIt(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR(), labels: []gh.Label{{Name: "bug", Color: "d73a4a"}}}
	m := scenarioModel(t, f)

	m = run(t, m, "2")
	if m.tab != tabRepos {
		t.Fatalf("precondition: tab = %d, want the Repos tab", m.tab)
	}

	m = run(t, m, "enter")
	if !strings.Contains(content(m), "state: open") {
		t.Fatalf("precondition: the detail view did not open:\n%s", content(m))
	}

	m = run(t, m, "l", "space", "enter")

	if len(f.pr.Labels) != 1 || f.pr.Labels[0].Name != "bug" {
		t.Fatalf("labels = %+v, want bug applied", f.pr.Labels)
	}
	if got := content(m); !strings.Contains(got, "bug") {
		t.Errorf("the view does not show the new label:\n%s", got)
	}
}

func TestCommentingOnADiffLineFromTheWorkBoardShowsTheThread(t *testing.T) {
	f := &scenarioSource{
		pr: scenarioPR(),
		files: []gh.FileDiff{{
			Path: "main.go", Status: gh.FileModified, Additions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -1,1 +1,1 @@",
				Lines: []gh.DiffLine{
					{Kind: gh.LineAdded, Text: "+package main", NewLine: 1},
				},
			}},
		}},
	}
	m := scenarioModel(t, f)
	if m.tab != tabWork {
		t.Fatalf("precondition: tab = %d, want the board", m.tab)
	}

	// The board's own d opens the diff too, so enter is checked on its own.
	m = run(t, m, "enter") // the card under the cursor -> the detail view
	// The card carries the number too; the state line is detail's alone.
	if !strings.Contains(content(m), "state: open") {
		t.Fatalf("the detail view did not open:\n%s", content(m))
	}

	m = run(t, m, "d") // the detail view -> the diff
	m = run(t, m, "j") // onto a line that can carry a comment
	// An empty body is not sent, so type one character.
	m = run(t, m, "c", "n", "ctrl+s")

	if len(f.posted) != 1 {
		t.Fatalf("posted = %+v, want one comment", f.posted)
	}
	if f.posted[0].Path != "main.go" || f.posted[0].Line != 1 {
		t.Errorf("posted at %s:%d, want main.go:1", f.posted[0].Path, f.posted[0].Line)
	}
	// The author is drawn by the thread row alone.
	if got := content(m); !strings.Contains(got, "kukv") {
		t.Errorf("the view does not show the posted thread:\n%s", got)
	}
}

// TestOpeningTheChecksFromTheReposTabNamesTheRepository covers a bug that
// only showed on a real terminal: the Repos tab handed out a ref with no
// repository, and the checks view drew a bare " #12".
func TestOpeningTheChecksFromTheReposTabNamesTheRepository(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR()}
	m := scenarioModel(t, f)

	m = run(t, m, "2")
	if m.tab != tabRepos {
		t.Fatalf("precondition: tab = %d, want the Repos tab", m.tab)
	}
	// The ref borrows the header's name, so the name has to have arrived.
	if !strings.Contains(content(m), "kukv/demo") {
		t.Fatalf("precondition: the Repos header has no repository name:\n%s", content(m))
	}

	m = run(t, m, "s")
	// The summary line is the checks view's alone.
	if !strings.Contains(content(m), "0 failing") {
		t.Fatalf("precondition: the checks view did not open:\n%s", content(m))
	}

	if got := content(m); !strings.Contains(got, "kukv/demo #12") {
		t.Errorf("the checks title does not name the repository:\n%s", got)
	}
}

// The keys a user actually presses to grow the list. The repo package tests
// the dialog itself; what this covers is that a, the letters and enter reach
// the Repos tab through the root's key routing at all.
func TestAddingARepositoryFromTheReposTab(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR()}
	m := scenarioModelWithRepos(t, f, []string{"kukv/octoscope"})

	m = run(t, m, "2")
	if m.tab != tabRepos {
		t.Fatalf("precondition: tab = %d, want the Repos tab", m.tab)
	}
	if strings.Contains(content(m), "a/b") {
		t.Fatalf("precondition: a/b is listed before it was added:\n%s", content(m))
	}

	// Off the temporary row first: standing on it, a opens the dialog with
	// that repository's name already in the field.
	m = run(t, m, "h", "j")
	// Short on purpose: every letter schedules a debounce tick that run waits
	// out, and what this covers is the routing, not the name.
	m = run(t, m, "a", "a", "/", "b", "enter")

	if !slices.Contains(f.saved, "a/b") {
		t.Errorf("saved = %v, want a/b written out", f.saved)
	}
	if !strings.Contains(content(m), "a/b") {
		t.Errorf("the added repository never reached the sidebar:\n%s", content(m))
	}
}

// q quits and 1 and 2 switch tabs, and the root acts on all three before it
// hands a key to the tab. A repository whose name carries one of them -- and
// q is not rare -- would quit octoscope or jump to the board mid-word.
func TestTypingAQuitKeyIntoTheDialogTypesIt(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR()}
	m := scenarioModelWithRepos(t, f, []string{"kukv/octoscope"})

	m = run(t, m, "2", "h", "j")
	m = press(m, "a")

	for _, k := range []string{"q", "1", "/", "2"} {
		next, cmd := m.Update(key(k))
		if isQuit(cmd) {
			t.Fatalf("%q quit octoscope instead of reaching the field", k)
		}
		m = next.(Model)
		if m.tab != tabRepos {
			t.Fatalf("%q left the Repos tab", k)
		}
	}

	m = run(t, m, "enter")
	if !slices.Contains(f.saved, "q1/2") {
		t.Errorf("saved = %v, want the name that was typed", f.saved)
	}
}

// The same route for x, including the pane move h that has to reach the
// sidebar first.
func TestRemovingARepositoryFromTheReposTab(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR()}
	m := scenarioModelWithRepos(t, f, []string{"kukv/octoscope", "kukv/koto"})

	m = run(t, m, "2")
	if !strings.Contains(content(m), "kukv/koto") {
		t.Fatalf("precondition: kukv/koto is not listed to begin with:\n%s", content(m))
	}

	// kukv/demo is the working directory's repository and leads the list as a
	// temporary row, so the settings file's two are the second and third.
	m = run(t, m, "h", "j", "j", "x")

	if strings.Contains(content(m), "kukv/koto") {
		t.Errorf("x did not remove the row:\n%s", content(m))
	}
	if !strings.Contains(content(m), "kukv/octoscope") {
		t.Errorf("x took the wrong row:\n%s", content(m))
	}
	if !slices.Equal(f.saved, []string{"kukv/octoscope"}) {
		t.Errorf("saved = %v, want [kukv/octoscope]", f.saved)
	}
}
