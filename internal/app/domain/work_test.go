package domain_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

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
	w.SetSection(domain.SectionAssigned, []domain.WorkItem{{Ref: domain.ItemRef{Number: 7}}})

	if n := len(w.Section(domain.SectionAssigned)); n != 1 {
		t.Fatalf("assigned column holds %d items, want 1", n)
	}
	if got := w.Section(domain.SectionAssigned)[0].Ref.Number; got != 7 {
		t.Errorf("got #%d, want #7", got)
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

	if domain.WorkSectionCount != len(sections) {
		t.Fatalf("Work has %d slots, %d sections are declared", domain.WorkSectionCount, len(sections))
	}
	if got := len(domain.WorkSections()); got != len(sections) {
		t.Errorf("WorkSections() returns %d, %d sections are declared", got, len(sections))
	}

	// Ask the board for each section rather than comparing counts. Work keeps
	// its columns unexported, so the count above compares two declarations to
	// each other and would still agree if the array behind them were the wrong
	// width; this reaches the array, and a section past its end panics here.
	var w domain.Work
	for _, s := range sections {
		_ = w.Section(s)
	}
}

func TestSectionGivesBackWhatSetSectionPutIn(t *testing.T) {
	t.Parallel()

	var w domain.Work
	w.SetSection(domain.SectionAssigned, []domain.WorkItem{{Title: "assigned"}})
	w.SetSection(domain.SectionMentioned, []domain.WorkItem{{Title: "mentioned"}})

	if got := w.Section(domain.SectionAssigned); len(got) != 1 || got[0].Title != "assigned" {
		t.Errorf("the assigned column holds %v", got)
	}
	if got := w.Section(domain.SectionMentioned); len(got) != 1 || got[0].Title != "mentioned" {
		t.Errorf("the mentioned column holds %v", got)
	}
	if got := w.Section(domain.SectionReviewRequested); got != nil {
		t.Errorf("a column nothing was put in holds %v", got)
	}
}
