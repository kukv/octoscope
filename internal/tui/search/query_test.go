package search_test

import (
	"testing"

	"github.com/kukv/octoscope/internal/tui/search"
)

// GitHub rejects an empty search, so the untouched form still has to mean
// something. It means what the mockup shows selected: everything still open.
func TestTheUntouchedFormSearchesForWhatIsOpen(t *testing.T) {
	t.Parallel()

	var f search.Filters
	if got := f.Query(); got != "is:open" {
		t.Errorf("Query() = %q, want %q", got, "is:open")
	}
}

func TestEachFilterAddsItsOwnQualifier(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		set  func(search.Filters) search.Filters
		want string
	}{
		{
			"type",
			func(f search.Filters) search.Filters { return f.Cycle(search.FilterType) },
			"is:open is:pr",
		},
		{
			"org",
			func(f search.Filters) search.Filters { return f.Set(search.FilterOrg, "kukv") },
			"is:open org:kukv",
		},
		{
			"repo",
			func(f search.Filters) search.Filters { return f.Set(search.FilterRepo, "kukv/octoscope") },
			"is:open repo:kukv/octoscope",
		},
		{
			"author",
			func(f search.Filters) search.Filters { return f.Set(search.FilterAuthor, "kukv") },
			"is:open author:kukv",
		},
		{
			"label",
			func(f search.Filters) search.Filters { return f.Set(search.FilterLabel, "bug") },
			"is:open label:bug",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			if got := c.set(search.Filters{}).Query(); got != c.want {
				t.Errorf("Query() = %q, want %q", got, c.want)
			}
		})
	}
}

// A label with a space in it is one qualifier, not two.
func TestALabelWithASpaceIsQuoted(t *testing.T) {
	t.Parallel()

	f := search.Filters{}.Set(search.FilterLabel, "good first issue")
	if got := f.Query(); got != `is:open label:"good first issue"` {
		t.Errorf("Query() = %q, want the label quoted", got)
	}
}

// state:all means the user asked for both, so neither is:open nor is:closed
// belongs in the query.
func TestStateAllDropsTheStateQualifier(t *testing.T) {
	t.Parallel()

	f := search.Filters{}
	for range 2 { // open -> closed -> all
		f = f.Cycle(search.FilterState)
	}
	if got := f.Query(); got != "" {
		t.Errorf("Query() = %q, want it empty once nothing is being filtered", got)
	}
}

func TestCyclingATypedFilterDoesNothing(t *testing.T) {
	t.Parallel()

	f := search.Filters{}.Set(search.FilterOrg, "kukv")
	if got := f.Cycle(search.FilterOrg); got.Value(search.FilterOrg) != "kukv" {
		t.Errorf("Value() = %q, want cycling to leave a typed filter alone", got.Value(search.FilterOrg))
	}
}

func TestCyclingComesBackAround(t *testing.T) {
	t.Parallel()

	f := search.Filters{}
	first := f.Value(search.FilterState)
	for range len(search.FilterState.Choices()) {
		f = f.Cycle(search.FilterState)
	}
	if f.Value(search.FilterState) != first {
		t.Errorf("Value() = %q after a full turn, want %q", f.Value(search.FilterState), first)
	}
}
