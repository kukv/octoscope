package drawer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/app/domain"
)

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
