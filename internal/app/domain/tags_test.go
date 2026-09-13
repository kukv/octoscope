package domain_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
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
	domain.ReviewTarget{},
}

// TestNoDomainTypeCarriesASerialisationTag is the wall this package's
// neutrality stands on. A tag here means some wire format reached in and
// made the application's shape its own -- which is how Author, Label and
// Comment ended up being decoded straight into before this was written.
//
// The walk recurses into any field whose type is (or contains, through a
// pointer, slice or array) a struct, including anonymous ones declared
// inline in a field. A tag on a nested field is just as much a leak as one
// on a top-level field.
func TestNoDomainTypeCarriesASerialisationTag(t *testing.T) {
	t.Parallel()
	for _, v := range exported {
		typ := reflect.TypeOf(v)
		walkFields(t, typ, typ.Name(), map[reflect.Type]bool{})
	}
}

// walkFields reports every struct tag found under typ, prefixing each
// failure with path so it names where the offending field lives (e.g.
// "PR.Meta.ID", not just "ID"). seen guards against infinite recursion on a
// self-referential type.
func walkFields(t *testing.T, typ reflect.Type, path string, seen map[reflect.Type]bool) {
	t.Helper()
	for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	if seen[typ] {
		return
	}
	seen[typ] = true
	for i := range typ.NumField() {
		f := typ.Field(i)
		fieldPath := path + "." + f.Name
		if f.Tag != "" {
			t.Errorf("%s carries a struct tag %q", fieldPath, f.Tag)
		}
		walkFields(t, f.Type, fieldPath, seen)
	}
}

// TestTheTagListCoversEveryExportedStruct keeps the list above honest: a
// type added to the package without being added here would go unchecked.
//
// It parses each non-test source file with go/parser rather than matching
// against a regex, so a grouped declaration (type ( Foo struct {...} )) or a
// generic one (type Foo[T any] struct {...}) is found the same as any other.
func TestTheTagListCoversEveryExportedStruct(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || !ts.Name.IsExported() {
					continue
				}
				if _, ok := ts.Type.(*ast.StructType); ok {
					seen[ts.Name.Name] = true
				}
			}
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
