package drawer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
)

// TestTheMetaLineLeavesRoomForItsBadges is why it measures what is left
// rather than what it has spent: the caller clips the line to the pane, and a
// line that overran would be cut through a badge — a block of colour with
// half a name in it, which reads as a smear rather than a label.
func TestTheMetaLineLeavesRoomForItsBadges(t *testing.T) {
	it := domain.WorkItem{
		Ref:    domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 12},
		Author: "kukv",
		Head:   "feat/graph", Base: "main", Additions: 218, Deletions: 31,
		Labels: []domain.Label{{Name: "bug", Color: "d73a4a"}, {Name: "ci", Color: "d4c5f9"}},
	}
	// A line whose parts alone overrun is the caller's to cut, and cutting
	// through a branch name costs nothing. What must never happen is a badge
	// added on top of a line with no room for it. Every width from there up
	// to where both badges fit is walked: the budget has to be right at each
	// one, not just at the roomy end.
	bare := it
	bare.Labels = nil
	base := ansi.StringWidth(metaLine(bare, 200))

	for w := base; w <= base+30; w++ {
		if got := ansi.StringWidth(metaLine(it, w)); got > w {
			t.Errorf("width %d: the meta line is %d columns: %q", w, got, ansi.Strip(metaLine(it, w)))
		}
	}
}

// TestFailingChecksComeFirst is why the drawer sorts: a failure is the reason
// to look at the list at all, and the budget cuts the tail off.
func TestFailingChecksComeFirst(t *testing.T) {
	lines := checksPane(domain.WorkItem{
		Ref: domain.ItemRef{Kind: domain.ItemPR},
		Checks: domain.Checks{
			Total: 3, Passed: 1, Failed: 1, Running: 1, State: domain.CheckFailure,
			Runs: []domain.CheckRun{
				{Name: "build", State: domain.CheckSuccess},
				{Name: "lint", State: domain.CheckRunning},
				{Name: "test", State: domain.CheckFailure},
			},
		},
	}, 40)
	got := ansi.Strip(strings.Join(lines, "\n"))
	if strings.Index(got, "test") > strings.Index(got, "build") {
		t.Errorf("the failing check is listed after a passing one:\n%s", got)
	}
}

// TestALongChecksListIsCutWithACount keeps the drawer a fixed height: it is
// drawn under a list, and a repository with thirty checks must not push the
// key bar off the screen.
func TestALongChecksListIsCutWithACount(t *testing.T) {
	c := domain.Checks{Total: 12, Passed: 12, State: domain.CheckSuccess}
	for i := range 12 {
		c.Runs = append(c.Runs, domain.CheckRun{Name: fmt.Sprintf("job-%d", i), State: domain.CheckSuccess})
	}

	lines := checksPane(domain.WorkItem{Ref: domain.ItemRef{Kind: domain.ItemPR}, Checks: c}, 40)
	if want := checks + 2; len(lines) != want { // the checks, the count, the summary
		t.Errorf("the list is %d lines, want %d:\n%s", len(lines), want, strings.Join(lines, "\n"))
	}
	if got := ansi.Strip(lines[checks]); !strings.Contains(got, "9") {
		t.Errorf("the line after the list does not count what was left out: %q", got)
	}
}
