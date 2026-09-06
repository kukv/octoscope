package diff

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
)

type fakeSource struct {
	files []gh.FileDiff
	err   error
}

func (f *fakeSource) PRDiff(context.Context, string, int) ([]gh.FileDiff, error) {
	return f.files, f.err
}

// fixture is two files, so that moving between files is testable, with a
// second hunk in the first so that hunk movement is too.
func fixture() []gh.FileDiff {
	return []gh.FileDiff{
		{
			Path: "graph/walk.go", Status: gh.FileModified, Additions: 4, Deletions: 1,
			Hunks: []gh.Hunk{
				{
					Header: "@@ -12,7 +12,9 @@ func Walk(ctx context.Context, q string) error {",
					Lines: []gh.DiffLine{
						{Kind: gh.LineContext, OldLine: 12, NewLine: 12, Text: "\tctx, cancel := context.WithTimeout(ctx, d)"},
						{Kind: gh.LineRemoved, OldLine: 13, Text: "\tif depth == 0 {"},
						{Kind: gh.LineAdded, NewLine: 13, Text: "\tif depth <= 0 {"},
						{Kind: gh.LineAdded, NewLine: 14, Text: "\t\tdepth = defaultDepth"},
					},
				},
				{
					Header: "@@ -40,3 +42,4 @@ func helper() {",
					Lines: []gh.DiffLine{
						{Kind: gh.LineContext, OldLine: 40, NewLine: 42, Text: "\t_ = q"},
						{Kind: gh.LineAdded, NewLine: 43, Text: "\t_ = depth"},
					},
				},
			},
		},
		{
			Path: "logo.png", Status: gh.FileModified, Binary: true,
		},
	}
}

func loaded(t *testing.T, width, height int) Model {
	t.Helper()
	m := New(&fakeSource{files: fixture()},
		gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(diffMsg{ref: m.ref, files: fixture()})
	// The header's title and branches arrive with the review context, not
	// with the diff, so a view that has only the diff draws a header with
	// the number in it and nothing else. Tests that read the header have to
	// hand the context over as well; see withThreads.
	return m
}

// emptyDiff is a model that has landed with no files at all, so the
// no-changes note is what draws.
func emptyDiff(t *testing.T, width, height int) Model {
	t.Helper()
	m := New(&fakeSource{}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 129})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(diffMsg{ref: m.ref, files: nil})
	return m
}

// key builds the KeyPressMsg for a key name, matching the shape the app uses.
func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

func press(m Model, k string) Model {
	m, _ = m.Update(key(k))
	return m
}

func TestTheFirstFileIsShown(t *testing.T) {
	out := ansi.Strip(loaded(t, 120, 30).View())
	for _, want := range []string{"graph/walk.go", "logo.png", "func Walk", "if depth <= 0 {"} {
		if !strings.Contains(out, want) {
			t.Errorf("the view does not show %q:\n%s", want, out)
		}
	}
}

func TestBracketsMoveBetweenFiles(t *testing.T) {
	m := press(loaded(t, 120, 30), "]")
	if m.file != 1 {
		t.Fatalf("file = %d, want 1", m.file)
	}
	if !strings.Contains(ansi.Strip(m.View()), "binary file") {
		t.Errorf("the binary file does not say so:\n%s", ansi.Strip(m.View()))
	}
	m = press(m, "[")
	if m.file != 0 {
		t.Errorf("file = %d, want back at 0", m.file)
	}
}

func TestBracesMoveBetweenHunks(t *testing.T) {
	m := loaded(t, 120, 30)
	m = press(m, "}")
	if got := m.rows[m.row].hunk; got != 1 {
		t.Errorf("after } the cursor is in hunk %d, want 1", got)
	}
	m = press(m, "{")
	if got := m.rows[m.row].hunk; got != 0 {
		t.Errorf("after { the cursor is back in hunk %d, want 0", got)
	}
}

func TestTheCursorStaysInsideTheFile(t *testing.T) {
	m := loaded(t, 120, 30)
	for range 100 {
		m = press(m, "j")
	}
	if m.row >= len(m.rows) {
		t.Fatalf("row = %d, past the last of %d rows", m.row, len(m.rows))
	}
	for range 100 {
		m = press(m, "k")
	}
	if m.row < 0 {
		t.Fatalf("row = %d, before the first", m.row)
	}
}

func TestChangingFileResetsTheCursor(t *testing.T) {
	m := loaded(t, 120, 30)
	m = press(m, "j")
	m = press(m, "j")
	m = press(m, "]")
	if m.row != 0 {
		t.Errorf("row = %d after changing file, want 0", m.row)
	}
}

func TestEscAsksTheParentToClose(t *testing.T) {
	m := loaded(t, 120, 30)
	_, cmd := m.Update(key("esc"))
	if cmd == nil {
		t.Fatal("esc produced no command")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Errorf("esc produced %T, want ClosedMsg", cmd())
	}
}

// TestAnswersForAnotherPullRequestAreDropped mirrors the detail view: the
// request for the item the user just left is still running when the next one
// opens, and its answer must not land here.
func TestAnswersForAnotherPullRequestAreDropped(t *testing.T) {
	m := loaded(t, 120, 30)
	other := gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 999}
	before := len(m.files)
	m, _ = m.Update(diffMsg{ref: other, files: nil})
	if len(m.files) != before {
		t.Errorf("an answer for %v replaced this view's %d files", other, before)
	}
}

func TestTheDiffFitsTheTerminal(t *testing.T) {
	for _, width := range []int{80, 120} {
		for _, height := range []int{24, 40} {
			m := loaded(t, width, height)
			out := m.View()
			if got := len(strings.Split(out, "\n")); got > height {
				t.Errorf("the diff drew %d lines into a %dx%d terminal", got, width, height)
			}
			if !strings.Contains(ansi.Strip(out), ansi.Strip(m.keyBar())) {
				t.Errorf("the key bar was pushed off the screen:\n%s", ansi.Strip(out))
			}
		}
	}
}

func TestCurrentRowIsZeroBeforeAnythingLoads(t *testing.T) {
	m := New(&fakeSource{}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 1})
	if got := m.currentRow(); got != (row{}) {
		t.Errorf("currentRow on an empty model = %+v, want the zero row", got)
	}
}

func TestCurrentRowFollowsTheCursor(t *testing.T) {
	m := press(loaded(t, 120, 30), "}")
	if got := m.currentRow(); got.kind != rowHunkHeader || got.hunk != 1 {
		t.Errorf("currentRow after } = %+v, want the second hunk's header", got)
	}
}
