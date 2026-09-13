package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(b)
}

// gh label list asks GraphQL for CREATED_AT ascending; REST answers by name.
// Nothing between here and the label picker reorders the list, so leaving
// REST's order alone would silently reshuffle the picker for anyone on this
// backend. Sorting by id restores gh's order -- verified against all 83 of
// cli/cli's labels on 2026-09-13.
func TestLabelsComeBackInTheOrderTheyWereCreatedNotAlphabetically(t *testing.T) {
	t.Parallel()

	c, _ := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "labels.json"))
	})
	labels, err := c.ListLabels(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if len(labels) != 83 {
		t.Fatalf("labels = %d, want 83", len(labels))
	}
	if labels[0].Name != "bug" {
		t.Errorf("first label = %q, want bug (the oldest); alphabetical would be accessibility", labels[0].Name)
	}
	if labels[1].Name != "blocked" {
		t.Errorf("second label = %q, want blocked", labels[1].Name)
	}
	if labels[0].Color == "" {
		t.Error("label carries no colour; the picker draws one")
	}
}

// The picker shows 100 assignable users at most, the same ceiling the cli
// backend reads. Asking for the default 30 would hide most of a large team.
func TestAssigneesAskForAFullPageNotTheDefaultThirty(t *testing.T) {
	t.Parallel()

	c, got := serveREST(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, fixture(t, "assignees.json"))
	})
	logins, err := c.ListAssignees(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListAssignees: %v", err)
	}
	if len(logins) == 0 {
		t.Fatal("no logins parsed")
	}
	if logins[0] == "" {
		t.Error("first login is empty")
	}
	if per := (*got)[0].URL.Query().Get("per_page"); per != "100" {
		t.Errorf("per_page = %q, want 100", per)
	}
}

// REST answers labels by name, and gives one page at a time. A repository
// with more than a page of labels needs every page walked before the id sort
// runs, or the picker gets the alphabetical head of the set instead of gh's
// oldest 100 -- a different set of labels, not just a different order.
func TestListLabelsOfARepositoryWithMoreThanAPageReturnsTheOldestHundredNotTheAlphabeticalHundred(t *testing.T) {
	t.Parallel()

	// 105 labels named so that name order and index coincide (label000 ..
	// label104), but ids run the other way (label104 is id 0, the oldest).
	// Reading only the first, alphabetical page would give label000..label099;
	// the true oldest 100 spans both pages and starts at label104.
	const total = 105
	type entry struct {
		Name  string `json:"name"`
		Color string `json:"color"`
		ID    int64  `json:"id"`
	}
	all := make([]entry, total)
	for i := range all {
		all[i] = entry{Name: fmt.Sprintf("label%03d", i), Color: "ffffff", ID: int64(total - 1 - i)}
	}

	var reqs int
	c, _ := serveREST(t, func(w http.ResponseWriter, r *http.Request) {
		reqs++
		page := all[:pageSize]
		if r.URL.Query().Get("page") == "2" {
			page = all[pageSize:]
		} else {
			w.Header().Set("Link", `<http://`+r.Host+`/repos/o/r/labels?per_page=100&page=2>; rel="next"`)
		}
		body, err := json.Marshal(page)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	})

	labels, err := c.ListLabels(context.Background(), "o/r")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	if reqs != 2 {
		t.Fatalf("requests = %d, want 2 (both pages read)", reqs)
	}
	if len(labels) != pageSize {
		t.Fatalf("labels = %d, want %d (gh's own ceiling)", len(labels), pageSize)
	}
	if labels[0].Name != "label104" {
		t.Errorf("oldest label = %q, want label104; label000 would be the alphabetical answer instead", labels[0].Name)
	}
}
