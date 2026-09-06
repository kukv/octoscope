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
