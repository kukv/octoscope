package gh_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

func TestWorkSectionsCoversEveryColumn(t *testing.T) {
	t.Parallel()

	got := gh.WorkSections()
	want := []gh.WorkSection{
		gh.SectionReviewRequested,
		gh.SectionYourPRs,
		gh.SectionAssigned,
		gh.SectionMentioned,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sections, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("section %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestWorkIndexesBySection(t *testing.T) {
	t.Parallel()

	var w gh.Work
	w[gh.SectionAssigned] = []gh.WorkItem{{Ref: gh.ItemRef{Number: 7}}}

	if n := len(w[gh.SectionAssigned]); n != 1 {
		t.Fatalf("assigned column holds %d items, want 1", n)
	}
	if got := w[gh.SectionAssigned][0].Ref.Number; got != 7 {
		t.Errorf("got #%d, want #7", got)
	}
}

func TestParseItemState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state string
		want  gh.ItemState
	}{
		{"OPEN", gh.StateOpen},
		{"CLOSED", gh.StateClosed},
		{"MERGED", gh.StateMerged},
		// gh's REST output lower-cases what GraphQL sends in capitals.
		{"open", gh.StateOpen},
		{"merged", gh.StateMerged},
		{"", gh.StateClosed},
		{"SOMETHING_NEW", gh.StateClosed},
	}

	for _, tt := range tests {
		if got := gh.ParseItemState(tt.state); got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestParseReviewDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		decision string
		want     gh.ReviewState
	}{
		{"APPROVED", gh.ReviewApproved},
		{"CHANGES_REQUESTED", gh.ReviewChangesRequested},
		{"REVIEW_REQUIRED", gh.ReviewRequired},
		{"", gh.ReviewNone},
		{"SOMETHING_NEW", gh.ReviewNone},
	}

	for _, tt := range tests {
		if got := gh.ParseReviewDecision(tt.decision); got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.decision, got, tt.want)
		}
	}
}

// Work is indexed by WorkSection. A fifth column added without widening Work
// panics at run time on the first item that lands in it -- there is no
// compiler error to catch it.
//
// The section list here is written out by hand on purpose. Taking it from
// WorkSections() would give both sides of the comparison the same source and
// the test could not fail, which is the shape .claude/rules/testing.md warns
// about: a column added to the enum and not to this list is what makes it go
// red, and that red is the reminder.
func TestEverySectionConstantIsASlotInWork(t *testing.T) {
	t.Parallel()

	sections := []gh.WorkSection{
		gh.SectionReviewRequested,
		gh.SectionYourPRs,
		gh.SectionAssigned,
		gh.SectionMentioned,
	}

	var w gh.Work
	if len(w) != len(sections) {
		t.Fatalf("Work has %d slots, %d sections are declared", len(w), len(sections))
	}
	if got := len(gh.WorkSections()); got != len(sections) {
		t.Errorf("WorkSections() returns %d, %d sections are declared", got, len(sections))
	}
	for _, s := range sections {
		if int(s) < 0 || int(s) >= len(w) {
			t.Errorf("section %d is not an index into Work (len %d)", s, len(w))
		}
	}
}
