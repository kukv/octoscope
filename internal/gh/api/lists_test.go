package api

import (
	"context"
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
func TestLabelsComeBackInTheOrderTheWereCreatedNotAlphabetically(t *testing.T) {
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
