package gql

import (
	"context"
	"regexp"
	"testing"
)

// The picker lists labels in the order the answer gives them. Sorting or
// deduplicating here would undo the order the document asks GitHub for.
func TestListLabelsKeepsTheRecordedOrder(t *testing.T) {
	t.Parallel()

	c, vars := fixedTransport(t, "testdata/labels.json")
	labels, err := c.ListLabels(context.Background(), "bright-room-sandbox/octoscope-test-repository")
	if err != nil {
		t.Fatalf("ListLabels: %v", err)
	}
	want := []string{"Priority: Low", "Close: Duplicate", "Need: Help Wanted"}
	if len(labels) < len(want) {
		t.Fatalf("got %d labels, want at least %d", len(labels), len(want))
	}
	for i, name := range want {
		if labels[i].Name != name {
			t.Errorf("labels[%d] = %q, want %q", i, labels[i].Name, name)
		}
		if labels[i].Color == "" {
			t.Errorf("labels[%d] has no color", i)
		}
	}
	if got := *vars; len(got) != 2 || got[0].Str != "bright-room-sandbox" || got[1].Str != "octoscope-test-repository" {
		t.Errorf("vars = %+v, want owner and name of the repository", got)
	}
}

// gh label list shows the oldest hundred labels, oldest first, and both
// backends send this one document. Dropping the order would hand back
// GitHub's default (by name), and a repository with more than a hundred
// labels would then lose a different set from the picker than gh does.
func TestLabelsDocumentAsksForTheOldestHundredOldestFirst(t *testing.T) {
	t.Parallel()

	doc := stripComments(labelsQuery)
	order := regexp.MustCompile(`orderBy:\s*\{\s*field:\s*CREATED_AT,\s*direction:\s*ASC\s*\}`)
	if !order.MatchString(doc) {
		t.Error("labels.graphql does not order by CREATED_AT ascending")
	}
	if !regexp.MustCompile(`labels\(first:\s*100\b`).MatchString(doc) {
		t.Error("labels.graphql does not ask for the first hundred")
	}
}

func TestListLabelsReportsTheTransportsError(t *testing.T) {
	t.Parallel()

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		return []byte(`{"data":{"repository":null}}`), context.Canceled
	}}
	if _, err := c.ListLabels(context.Background(), "o/r"); err == nil {
		t.Fatal("want the transport's error back")
	}
}
