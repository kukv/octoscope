package layout_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/tui/layout"
)

func TestJoinPanesRunsTheRuleDownTheTallerPane(t *testing.T) {
	t.Parallel()

	got := layout.JoinPanes([]string{"a"}, []string{"x", "y", "z"}, 10)
	if len(got) != 3 {
		t.Fatalf("got %d lines, want 3: %q", len(got), got)
	}
	for i, line := range got {
		if !strings.Contains(line, "│") {
			t.Errorf("line %d has no rule: %q", i, line)
		}
	}
}

func TestJoinPanesPutsTheRuleInTheSameColumnOnEveryLine(t *testing.T) {
	t.Parallel()

	// A short left line and a long one must not move the rule: the eye
	// reads the boundary as one straight line down the page.
	got := layout.JoinPanes([]string{"a", "あいうえ"}, []string{"x", "y"}, 10)
	want := -1
	for i, line := range got {
		before, _, found := strings.Cut(line, "│")
		if !found {
			t.Fatalf("line %d has no rule: %q", i, line)
		}
		at := ansi.StringWidth(before)
		if want == -1 {
			want = at
			continue
		}
		if at != want {
			t.Errorf("line %d puts the rule at column %d, want %d", i, at, want)
		}
	}
	if want != 10 {
		t.Errorf("the rule sits at column %d, want 10", want)
	}
}
