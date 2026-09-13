package domain_test

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
)

// exported is every struct this package publishes. The list is written by
// hand so that a new type has to be added here deliberately; the test below
// it fails when someone forgets.
var exported = []any{
	domain.Author{},
	domain.Label{},
	domain.Comment{},
	domain.PR{},
	domain.Issue{},
	domain.ItemRef{},
	domain.CheckRun{},
	domain.Checks{},
	domain.WorkItem{},
	domain.RepoCount{},
	domain.RepoCandidate{},
	domain.SavedQuery{},
	domain.LogLine{},
	domain.DiffLine{},
	domain.Hunk{},
	domain.FileDiff{},
	domain.MergeContext{},
	domain.ThreadComment{},
	domain.ReviewThread{},
	domain.PendingComment{},
	domain.ReviewContext{},
}

// TestNoDomainTypeCarriesASerialisationTag is the wall this package's
// neutrality stands on. A tag here means some wire format reached in and
// made the application's shape its own -- which is how Author, Label and
// Comment ended up being decoded straight into before this was written.
func TestNoDomainTypeCarriesASerialisationTag(t *testing.T) {
	t.Parallel()
	for _, v := range exported {
		typ := reflect.TypeOf(v)
		for i := range typ.NumField() {
			f := typ.Field(i)
			if f.Tag != "" {
				t.Errorf("%s.%s carries a struct tag %q", typ.Name(), f.Name, f.Tag)
			}
		}
	}
}

// TestTheTagListCoversEveryExportedStruct keeps the list above honest: a
// type added to the package without being added here would go unchecked.
func TestTheTagListCoversEveryExportedStruct(t *testing.T) {
	t.Parallel()
	decl := regexp.MustCompile(`(?m)^type ([A-Z]\w*) struct`)
	seen := map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range decl.FindAllStringSubmatch(string(raw), -1) {
			seen[m[1]] = true
		}
	}
	listed := map[string]bool{}
	for _, v := range exported {
		listed[reflect.TypeOf(v).Name()] = true
	}
	for name := range seen {
		if !listed[name] {
			t.Errorf("domain.%s is not in the list the tag test walks", name)
		}
	}
	for name := range listed {
		if !seen[name] {
			t.Errorf("the list names domain.%s, which this package does not declare", name)
		}
	}
}
