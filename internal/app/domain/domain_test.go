package domain_test

import (
	"reflect"
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
	w[domain.SectionAssigned] = []domain.WorkItem{{Ref: domain.ItemRef{Number: 7}}}

	if n := len(w[domain.SectionAssigned]); n != 1 {
		t.Fatalf("assigned column holds %d items, want 1", n)
	}
	if got := w[domain.SectionAssigned][0].Ref.Number; got != 7 {
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
