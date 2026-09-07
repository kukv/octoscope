package app

import (
	"context"
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
// next fetch sees it. app_test.go's fakeSource returns fixed values, which is
// enough for routing but cannot show a scenario reaching its end.
type scenarioSource struct {
	pr      gh.PR
	files   []gh.FileDiff
	labels  []gh.Label
	threads []gh.ReviewThread

	pendingID string
	posted    []gh.PendingComment
}

func (f *scenarioSource) ListWork(context.Context) (gh.Work, error) {
	var w gh.Work
	w[gh.SectionReviewRequested] = []gh.WorkItem{{
		Ref:       gh.ItemRef{Kind: gh.ItemPR, Number: f.pr.Number},
		Title:     f.pr.Title,
		Author:    f.pr.Author.Login,
		UpdatedAt: f.pr.UpdatedAt,
	}}
	return w, nil
}

func (f *scenarioSource) ListPRs(context.Context) ([]gh.PR, error)       { return []gh.PR{f.pr}, nil }
func (f *scenarioSource) ListIssues(context.Context) ([]gh.Issue, error) { return nil, nil }
func (f *scenarioSource) RepoName(context.Context) (string, error)       { return "kukv/demo", nil }

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
// usecase does, and the thread it created shows up in the next fetch.
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

var scenarioAt = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

func scenarioPR() gh.PR {
	return gh.PR{
		Number: 12, Title: "replace the renderer",
		Author: gh.Author{Login: "kukv"}, State: gh.StateOpen,
		UpdatedAt: scenarioAt, Body: "body",
	}
}

// run presses each key and lets whatever it started finish, so a scenario
// reads as the keys a user types. resolve is what makes that safe: it opens
// the batches the fetches come wrapped in and stops at the spinner's tick.
func run(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, cmd := pressCmd(m, k)
		m = resolve(t, next, cmd)
	}
	return m
}

// scenarioModel drives the root the way the terminal does: a size first, and
// then everything the first size started.
func scenarioModel(t *testing.T, f *scenarioSource) Model {
	t.Helper()
	i18n.SetLanguage(language.English)
	t.Cleanup(func() { i18n.SetLanguage(language.English) })

	m := New(f, Options{HasRepo: true})
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return resolve(t, next.(Model), cmd)
}

// TestClosingFromTheReposTabShowsTheNewState walks the keys a user actually
// presses: switch to Repos, open the row, ask to close it, confirm. Nothing
// but the key sequence drives it, so the list, the detail view and the root
// all have to agree for this to pass.
func TestClosingFromTheReposTabShowsTheNewState(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR()}
	m := scenarioModel(t, f)

	m = run(t, m, "2", "enter") // the Repos row -> the detail view
	if !strings.Contains(content(m), "#12") {
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

// TestPickingALabelFromTheDetailViewAppliesIt covers the picker end to end:
// the candidates are fetched, one is toggled, enter applies it, and the
// refetch that follows is what puts it on screen.
func TestPickingALabelFromTheDetailViewAppliesIt(t *testing.T) {
	f := &scenarioSource{pr: scenarioPR(), labels: []gh.Label{{Name: "bug", Color: "d73a4a"}}}
	m := scenarioModel(t, f)

	m = run(t, m, "2", "enter", "l", "space", "enter")

	if len(f.pr.Labels) != 1 || f.pr.Labels[0].Name != "bug" {
		t.Fatalf("labels = %+v, want bug applied", f.pr.Labels)
	}
	if got := content(m); !strings.Contains(got, "bug") {
		t.Errorf("the view does not show the new label:\n%s", got)
	}
}

// TestCommentingOnADiffLineShowsTheThread is the scenario the whole
// remediation started from: the line comment that never worked. It crosses
// three views (board, detail, diff) on key presses alone.
func TestCommentingOnADiffLineShowsTheThread(t *testing.T) {
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

	m = run(t, m, "2", "enter", "d") // Repos -> the detail view -> the diff
	m = run(t, m, "j")               // onto a line that can carry a comment
	// "n" is one character of body: an empty one is not sent. key() puts a
	// single character in Text, which is what the textarea inserts.
	m = run(t, m, "c", "n", "ctrl+s")

	if len(f.posted) != 1 {
		t.Fatalf("posted = %+v, want one comment", f.posted)
	}
	if f.posted[0].Path != "main.go" || f.posted[0].Line != 1 {
		t.Errorf("posted at %s:%d, want main.go:1", f.posted[0].Path, f.posted[0].Line)
	}
	// The thread row carries its author, which nothing else on this screen
	// does -- a one-character body would match anywhere.
	if got := content(m); !strings.Contains(got, "kukv") {
		t.Errorf("the view does not show the posted thread:\n%s", got)
	}
}
