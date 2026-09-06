package diff

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/i18n"
)

type fakeSource struct {
	files  []gh.FileDiff
	err    error
	review gh.ReviewContext
}

func (f *fakeSource) PRDiff(context.Context, string, int) ([]gh.FileDiff, error) {
	return f.files, f.err
}

func (f *fakeSource) PRReviewContext(context.Context, string, int) (gh.ReviewContext, error) {
	return f.review, nil
}

// StartReview, AddReviewThread, DiscardReview, SubmitReview and
// SubmitNewReview are stubs so fakeSource satisfies Source; recordingSource
// (comment_test.go) overrides the ones a comment test needs to assert
// against.
func (f *fakeSource) StartReview(string) (string, error) { return "", nil }

func (f *fakeSource) AddReviewThread(string, gh.PendingComment) error { return nil }

func (f *fakeSource) DiscardReview(string) error { return nil }

func (f *fakeSource) SubmitReview(string, gh.ReviewEvent, string) error { return nil }

func (f *fakeSource) SubmitNewReview(string, gh.ReviewEvent, string) error { return nil }

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

// noPatchDiff is a model whose one file is too large for GitHub to have sent
// a patch for (the files-API fallback's own failure mode), so the
// patch-omitted note is what draws instead of the binary one.
func noPatchDiff(t *testing.T, width, height int) Model {
	t.Helper()
	files := []gh.FileDiff{{Path: "vendor/bundle.js", Status: gh.FileModified, PatchOmitted: true}}
	m := New(&fakeSource{files: files}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 130})
	m, _ = m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	m, _ = m.Update(diffMsg{ref: m.ref, files: files})
	return m
}

// key builds the KeyPressMsg for a key name, matching the shape the app uses.
func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
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

// TestAFileWithNoPatchSaysSo guards the files-API fallback's own failure
// mode: a file too large for GitHub to send a patch for must not read as a
// file with no changes.
func TestAFileWithNoPatchSaysSo(t *testing.T) {
	out := ansi.Strip(noPatchDiff(t, 120, 30).View())
	if !strings.Contains(out, i18n.T("diff.patch_omitted")) {
		t.Errorf("the view does not say the patch was omitted:\n%s", out)
	}
	if strings.Contains(out, i18n.T("diff.binary")) {
		t.Errorf("a file with no patch must not read as binary:\n%s", out)
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

// TestOpeningADiffParksOnTheFirstLine guards against c-silent: the cursor
// used to start on row 0, which buildRows always fills with a rowHunkHeader,
// and a hunk header has no line to comment on. The fixture check is what
// keeps this test honest -- if the fixture ever stopped starting with a
// hunk header, the assertion below could pass by accident.
func TestOpeningADiffParksOnTheFirstLine(t *testing.T) {
	m := loaded(t, 120, 30)
	if got := m.rows[0].kind; got != rowHunkHeader {
		t.Fatalf("fixture's first row is %v, not rowHunkHeader -- this test proves nothing", got)
	}
	if got := m.currentRow().kind; got != rowLine {
		t.Errorf("cursor parked on %v after opening, want rowLine", got)
	}
}

// TestChangingFileParksOnTheFirstLine is ] and ['s share of the same fix:
// moveFile rebuilds m.rows for the new file, and the cursor must land on a
// line there too, not on the new file's own hunk header.
func TestChangingFileParksOnTheFirstLine(t *testing.T) {
	files := manyFilesFixture()
	m := New(&fakeSource{files: files}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m, _ = m.Update(diffMsg{ref: m.ref, files: files})
	if got := m.rows[0].kind; got != rowHunkHeader {
		t.Fatalf("fixture's first row is %v, not rowHunkHeader -- this test proves nothing", got)
	}
	m = press(m, "]")
	if got := m.currentRow().kind; got != rowLine {
		t.Errorf("cursor parked on %v after changing file, want rowLine", got)
	}
}

// TestNoCommentableLineLeavesCursorOnTheNote covers the fallback: a binary
// file, a file whose patch GitHub omitted, and an empty diff each render a
// single rowNote and nothing else, so there is no line to park on.
func TestNoCommentableLineLeavesCursorOnTheNote(t *testing.T) {
	m := press(loaded(t, 120, 30), "]") // logo.png, binary
	if got := m.currentRow().kind; got != rowNote {
		t.Errorf("cursor on %v for a binary file, want rowNote", got)
	}
	_ = m.View() // must not panic

	if got := noPatchDiff(t, 120, 30).currentRow().kind; got != rowNote {
		t.Errorf("cursor on %v for a file with no patch, want rowNote", got)
	}
	if got := emptyDiff(t, 120, 30).currentRow().kind; got != rowNote {
		t.Errorf("cursor on %v for an empty diff, want rowNote", got)
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
	if got := m.currentRow(); got.kind != rowHunkHeader || got.hunk != 0 || got.text != "" {
		t.Errorf("currentRow on an empty model = %+v, want the zero row", got)
	}
}

func TestCurrentRowFollowsTheCursor(t *testing.T) {
	m := press(loaded(t, 120, 30), "}")
	if got := m.currentRow(); got.kind != rowHunkHeader || got.hunk != 1 {
		t.Errorf("currentRow after } = %+v, want the second hunk's header", got)
	}
}

// TestADiffFailureGoesToTheParentsErrorScreen is the diff fetch's own
// failure, which leaves nothing to show: it still becomes ErrorMsg for the
// parent's whole-screen error view.
func TestADiffFailureGoesToTheParentsErrorScreen(t *testing.T) {
	m := New(&fakeSource{}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, cmd := m.Update(errMsg{ref: m.ref, err: errors.New("boom")})
	if cmd == nil {
		t.Fatal("a diff failure produced no command")
	}
	got, ok := cmd().(ErrorMsg)
	if !ok {
		t.Fatalf("a diff failure produced %T, want ErrorMsg", cmd())
	}
	if got.Err.Error() != "boom" {
		t.Errorf("ErrorMsg.Err = %q, want %q", got.Err, "boom")
	}
}

// TestAReviewFailureLeavesTheDiffReadable is the review context's own
// failure: the diff itself is fine, so it must not escalate to the parent's
// error screen, and the diff must still be on screen alongside the failure.
func TestAReviewFailureLeavesTheDiffReadable(t *testing.T) {
	m := loaded(t, 120, 30)
	m, cmd := m.Update(reviewErrMsg{ref: m.ref, err: errors.New("boom from github")})
	if cmd != nil {
		if _, ok := cmd().(ErrorMsg); ok {
			t.Fatal("a review failure escalated to the parent's error screen")
		}
	}
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "func Walk") {
		t.Errorf("the diff is no longer readable after a review failure:\n%s", out)
	}
	if !strings.Contains(out, "boom from github") {
		t.Errorf("the review failure is not shown:\n%s", out)
	}
}

// TestAReviewFailureForAnotherPullRequestIsDropped mirrors
// TestAnswersForAnotherPullRequestAreDropped: the request for the item the
// user just left is still in flight, and its failure must not land here.
func TestAReviewFailureForAnotherPullRequestIsDropped(t *testing.T) {
	m := loaded(t, 120, 30)
	other := gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 999}
	m, _ = m.Update(reviewErrMsg{ref: other, err: errors.New("boom")})
	if strings.Contains(ansi.Strip(m.View()), "boom") {
		t.Errorf("a review failure for %v was shown in this view's footer", other)
	}
}

// TestAMultiLineReviewFailureStaysOnOneRow is reviewErrLine's share of the
// newline bug: gh and GitHub both return multi-line error text (a GraphQL
// error array, a wrapped stderr), and the failure line has the same one-row
// invariant as a thread comment.
func TestAMultiLineReviewFailureStaysOnOneRow(t *testing.T) {
	single := loaded(t, 120, 30)
	single, _ = single.Update(reviewErrMsg{ref: single.ref, err: errors.New("boom from github: retry later")})
	want := len(strings.Split(single.View(), "\n"))

	multi := loaded(t, 120, 30)
	multi, _ = multi.Update(reviewErrMsg{ref: multi.ref, err: errors.New("boom from github\n\nretry later")})
	got := len(strings.Split(multi.View(), "\n"))

	if got != want {
		t.Errorf("a multi-line review failure drew %d rows, want %d", got, want)
	}
	if !strings.Contains(ansi.Strip(multi.View()), "boom from github") {
		t.Errorf("the failure text is missing:\n%s", ansi.Strip(multi.View()))
	}
}

// TestTheDiffFitsTheTerminalWithAReviewFailure extends
// TestTheDiffFitsTheTerminal: the failure line must come out of the pane's
// height budget, the same way the key bar does, so it must never push the key
// bar off the bottom.
func TestTheDiffFitsTheTerminalWithAReviewFailure(t *testing.T) {
	for _, width := range []int{80, 120} {
		for _, height := range []int{24, 40} {
			m := loaded(t, width, height)
			m, _ = m.Update(reviewErrMsg{ref: m.ref, err: errors.New("boom")})
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

func TestTheSidebarFoldsAtNarrowWidths(t *testing.T) {
	tests := []struct {
		name    string
		width   int
		sidebar bool
	}{
		{"wide enough for both", 120, true},
		{"exactly at the threshold", 100, true},
		{"one column short", 99, false},
		{"the narrowest terminal", 80, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := loaded(t, tt.width, 30)
			if got := m.showSidebar(); got != tt.sidebar {
				t.Errorf("showSidebar() = %v at %d columns, want %v", got, tt.width, tt.sidebar)
			}
			out := ansi.Strip(m.View())
			// The file being read is named either way: in the list when
			// there is one, in the header when there is not.
			if !strings.Contains(out, "graph/walk.go") {
				t.Errorf("the file being read is not named at %d columns:\n%s", tt.width, out)
			}
			if !tt.sidebar && strings.Contains(out, "logo.png") {
				t.Errorf("the folded sidebar still lists other files at %d columns:\n%s", tt.width, out)
			}
		})
	}
}

// TestHAndLDoNotSelectAFoldedSidebar guards the "nowhere to move to" rule:
// below the threshold h must not put the cursor in a pane that is not drawn.
func TestHAndLDoNotSelectAFoldedSidebar(t *testing.T) {
	m := press(loaded(t, 80, 30), "h")
	if m.sidebar {
		t.Error("h selected the sidebar even though it is folded at 80 columns")
	}
}

// TestNarrowingTheTerminalLeavesTheSidebar covers the resize case: a cursor
// already in the sidebar above the threshold must not be left pointing at a
// pane that stops being drawn once the terminal narrows.
func TestNarrowingTheTerminalLeavesTheSidebar(t *testing.T) {
	m := press(loaded(t, 120, 30), "h")
	if !m.sidebar {
		t.Fatal("h did not select the sidebar at 120 columns")
	}
	m, _ = m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if m.sidebar {
		t.Error("the cursor is still in the sidebar after narrowing below the threshold")
	}
}

// manyFilesFixture is more files than a typical terminal's sidebar has room
// for, so moving the selection past the bottom exercises the sidebar's own
// scroll.
func manyFilesFixture() []gh.FileDiff {
	files := make([]gh.FileDiff, 20)
	for i := range files {
		files[i] = gh.FileDiff{
			Path: fmt.Sprintf("pkg/file%02d.go", i), Status: gh.FileModified,
			Additions: 1,
			Hunks: []gh.Hunk{{
				Header: "@@ -1,1 +1,1 @@",
				Lines:  []gh.DiffLine{{Kind: gh.LineAdded, NewLine: 1, Text: "x"}},
			}},
		}
	}
	return files
}

// TestTheSidebarScrollsToKeepTheSelectionVisible is the sidebar's own
// follow, deferred from Task 5: on a pull request touching more files than
// fit, moving the selection past the bottom must not run it off screen.
func TestTheSidebarScrollsToKeepTheSelectionVisible(t *testing.T) {
	files := manyFilesFixture()
	m := New(&fakeSource{files: files}, gh.ItemRef{Kind: gh.ItemPR, Repo: "kukv/koto", Number: 128})
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 15})
	m, _ = m.Update(diffMsg{ref: m.ref, files: files})
	m.sidebar = true
	for range files {
		m = press(m, "]")
	}
	if m.file != len(files)-1 {
		t.Fatalf("file = %d, want %d", m.file, len(files)-1)
	}
	out := ansi.Strip(m.View())
	want := files[len(files)-1].Path
	if !strings.Contains(out, want) {
		t.Errorf("the selected file %q scrolled off screen:\n%s", want, out)
	}
}
