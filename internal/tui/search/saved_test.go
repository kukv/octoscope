package search

import (
	"testing"

	"github.com/kukv/octoscope/internal/usecase"
)

// upsert must not write into the caller's backing array: handleNameKey hands
// it m.saved and then starts saveQueries(m.src, m.saved) as a tea.Cmd, which
// reads that same slice from a goroutine. A destructive replace racing with
// that read is a data race the moment the same name is saved twice in a
// row; removeSaved (below it in saved.go) already avoids this with
// slices.Clone.
func TestUpsertDoesNotMutateTheCallersSlice(t *testing.T) {
	t.Parallel()

	orig := []usecase.SavedQuery{{Name: "mine", Query: "is:open"}}
	got := upsert(orig, usecase.SavedQuery{Name: "mine", Query: "is:closed"})

	if orig[0].Query != "is:open" {
		t.Errorf("upsert mutated the caller's slice: %+v", orig)
	}
	if len(got) != 1 || got[0].Query != "is:closed" {
		t.Errorf("got = %+v, want the replacement", got)
	}
}
