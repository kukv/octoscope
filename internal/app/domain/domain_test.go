package domain_test

import (
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// What the user must act on takes the whole screen; everything else costs
// them a line. Getting this wrong either hides a failure they can fix or
// throws a board away over a 502.
func TestIsFatalOnlyForWhatTheUserMustActOn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"backend unavailable", domain.ErrBackendUnavailable, true},
		{"not signed in", fmt.Errorf("gh pr list: %w", domain.ErrUnauthenticated), true},
		{"GitHub did not answer", fmt.Errorf("gh pr list: %w", domain.ErrTransient), false},
		{"anything else", errors.New("gh: HTTP 404"), false},
	}
	for _, tt := range tests {
		if got := domain.IsFatal(tt.err); got != tt.want {
			t.Errorf("%s: IsFatal = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestWorkSectionsCoversEveryColumn(t *testing.T) {
	t.Parallel()

	got := domain.WorkSections()
	want := []domain.WorkSection{
		domain.SectionReviewRequested,
		domain.SectionYourPRs,
		domain.SectionAssigned,
		domain.SectionMentioned,
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

	var w domain.Work
	w[domain.SectionAssigned] = []domain.WorkItem{{Ref: domain.ItemRef{Number: 7}}}

	if n := len(w[domain.SectionAssigned]); n != 1 {
		t.Fatalf("assigned column holds %d items, want 1", n)
	}
	if got := w[domain.SectionAssigned][0].Ref.Number; got != 7 {
		t.Errorf("got #%d, want #7", got)
	}
}

func TestParseItemState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		state string
		want  domain.ItemState
	}{
		{"OPEN", domain.StateOpen},
		{"CLOSED", domain.StateClosed},
		{"MERGED", domain.StateMerged},
		// gh's REST output lower-cases what GraphQL sends in capitals.
		{"open", domain.StateOpen},
		{"merged", domain.StateMerged},
		{"", domain.StateClosed},
		{"SOMETHING_NEW", domain.StateClosed},
	}

	for _, tt := range tests {
		if got := domain.ParseItemState(tt.state); got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.state, got, tt.want)
		}
	}
}

func TestParseReviewDecision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		decision string
		want     domain.ReviewState
	}{
		{"APPROVED", domain.ReviewApproved},
		{"CHANGES_REQUESTED", domain.ReviewChangesRequested},
		{"REVIEW_REQUIRED", domain.ReviewRequired},
		{"", domain.ReviewNone},
		{"SOMETHING_NEW", domain.ReviewNone},
	}

	for _, tt := range tests {
		if got := domain.ParseReviewDecision(tt.decision); got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.decision, got, tt.want)
		}
	}
}

// A section added without widening Work panics at run time with no compiler
// error. The list below is written by hand (.claude/rules/testing.md).
func TestEverySectionConstantIsASlotInWork(t *testing.T) {
	t.Parallel()

	sections := []domain.WorkSection{
		domain.SectionReviewRequested,
		domain.SectionYourPRs,
		domain.SectionAssigned,
		domain.SectionMentioned,
	}

	var w domain.Work
	if len(w) != len(sections) {
		t.Fatalf("Work has %d slots, %d sections are declared", len(w), len(sections))
	}
	if got := len(domain.WorkSections()); got != len(sections) {
		t.Errorf("WorkSections() returns %d, %d sections are declared", got, len(sections))
	}
	for _, s := range sections {
		if int(s) < 0 || int(s) >= len(w) {
			t.Errorf("section %d is not an index into Work (len %d)", s, len(w))
		}
	}
}

// The settings file's shape belongs to internal/app/config; this type
// is what the application is written in terms of. A tag here would mean
// the two had been merged back together.
func TestSavedQueryCarriesNoSerialisationTags(t *testing.T) {
	typ := reflect.TypeOf(domain.SavedQuery{})
	for i := range typ.NumField() {
		if tag := typ.Field(i).Tag; tag != "" {
			t.Errorf("SavedQuery.%s carries a struct tag %q", typ.Field(i).Name, tag)
		}
	}
}
