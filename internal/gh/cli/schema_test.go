package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

// GraphQL validates a document against the schema before running any of it,
// so one misspelled field fails the whole request -- not the branch that
// selects it. A fake that answers with hand-written JSON never sees the
// document, which is how review.graphql shipped selecting diffSide on
// PullRequestReviewComment, a field that type does not have, and failed every
// time it ran. These tests walk each document against a recorded copy of the
// schema instead. See testdata/README.md for how to record it.

type schemaType struct {
	Kind          string            `json:"kind"`
	PossibleTypes []string          `json:"possibleTypes"`
	Fields        map[string]string `json:"fields"`
}

func loadSchema(t *testing.T) map[string]schemaType {
	t.Helper()

	raw, err := os.ReadFile("testdata/schema.json")
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var s map[string]schemaType
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	return s
}

func TestEveryFieldTheDocumentsSelectExistsInTheSchema(t *testing.T) {
	t.Parallel()

	docs := map[string]string{
		"work.graphql":           workQuery,
		"review.graphql":         reviewContextQuery,
		"start_review.graphql":   startReviewMutation,
		"add_thread.graphql":     addThreadMutation,
		"submit_review.graphql":  submitReviewMutation,
		"review_at_once.graphql": reviewAtOnceMutation,
		"discard_review.graphql": discardReviewMutation,
	}
	schema := loadSchema(t)
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			for _, err := range checkDocument(doc, schema) {
				t.Errorf("%s: %v", name, err)
			}
		})
	}
}

// TestTheWalkReachesTheNestedSelections guards the guard: a walk that stops
// at the first level would pass every document above without looking at the
// place the diffSide bug lived.
func TestTheWalkReachesTheNestedSelections(t *testing.T) {
	t.Parallel()

	schema := loadSchema(t)
	cases := map[string]string{
		"nested in a query": `query { repository(owner: "o", name: "n") {
			pullRequest(number: 1) { reviews(first: 1) { nodes { id nope } } } } }`,
		"inside a fragment": `query { search(type: ISSUE, first: 1, query: "q") { nodes { ...F } } }
			fragment F on SearchResultItem { ... on CheckRun { nope } }`,
		"inside a mutation payload": `mutation { addPullRequestReview(input: {}) {
			pullRequestReview { nope } } }`,
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			errs := checkDocument(doc, schema)
			if len(errs) == 0 {
				t.Fatal("the walk accepted a field that does not exist")
			}
			if !strings.Contains(fmt.Sprint(errs), "nope") {
				t.Errorf("the walk complained about something else: %v", errs)
			}
		})
	}
}

// checkDocument reports every selected field the schema does not have.
func checkDocument(doc string, schema map[string]schemaType) []error {
	toks := tokenize(doc)
	w := &walker{schema: schema, fragments: map[string]fragment{}}
	w.collectFragments(toks)
	w.walkOperations(toks)
	return w.errs
}

type fragment struct {
	on   string
	body []string // tokens between the braces
}

type walker struct {
	schema    map[string]schemaType
	fragments map[string]fragment
	errs      []error
}

func (w *walker) fail(format string, a ...any) {
	w.errs = append(w.errs, fmt.Errorf(format, a...))
}

// typeOf looks a type up, reporting the recording command once rather than
// treating an unrecorded type as a schema error.
func (w *walker) typeOf(name string) (schemaType, bool) {
	t, ok := w.schema[name]
	if !ok {
		w.fail("type %s is not in testdata/schema.json; add it to the command in testdata/README.md", name)
	}
	return t, ok
}

func (w *walker) collectFragments(toks []string) {
	for i := 0; i < len(toks); i++ {
		if toks[i] != "fragment" || i+4 >= len(toks) {
			continue
		}
		name, on := toks[i+1], toks[i+3]
		open := i + 4
		if toks[open] != "{" {
			continue
		}
		end := matchBrace(toks, open)
		w.fragments[name] = fragment{on: on, body: toks[open+1 : end]}
	}
}

func (w *walker) walkOperations(toks []string) {
	for i := 0; i < len(toks); i++ {
		root := ""
		switch toks[i] {
		case "query":
			root = "Query"
		case "mutation":
			root = "Mutation"
		default:
			continue
		}
		if i+1 >= len(toks) || toks[i+1] != "{" {
			continue
		}
		end := matchBrace(toks, i+1)
		w.walkSelection(toks[i+2:end], root)
		i = end
	}
}

// walkSelection checks one selection set -- the tokens between a matched pair
// of braces -- against the type it is selected from.
func (w *walker) walkSelection(toks []string, typeName string) {
	parent, ok := w.typeOf(typeName)
	if !ok {
		return
	}
	// An interface carries fields and they can be selected directly; a union
	// carries none, so everything but __typename has to go in a fragment.
	union := parent.Kind == "UNION"

	for i := 0; i < len(toks); i++ {
		switch toks[i] {
		case "...":
			i = w.walkSpread(toks, i, parent, typeName)
			continue
		case "{", "}":
			continue
		}

		name := toks[i]
		// An alias renames the field; the schema knows only the field.
		if i+1 < len(toks) && toks[i+1] == ":" {
			i += 2
			if i >= len(toks) {
				return
			}
			name = toks[i]
		}

		// __typename is selectable on every type, unions included.
		if name == "__typename" {
			continue
		}
		if union {
			w.fail("%s is a union, so %s has to be selected inside a fragment", typeName, name)
			continue
		}
		fieldType, ok := parent.Fields[name]
		if !ok {
			w.fail("%s has no field %s", typeName, name)
			continue
		}
		if i+1 < len(toks) && toks[i+1] == "{" {
			end := matchBrace(toks, i+1)
			w.walkSelection(toks[i+2:end], fieldType)
			i = end
		}
	}
}

// walkSpread handles both "... on Type { ... }" and "...FragmentName", and
// returns the index of the last token it consumed.
func (w *walker) walkSpread(toks []string, i int, parent schemaType, typeName string) int {
	if i+1 >= len(toks) {
		w.fail("%s: a spread with nothing after it", typeName)
		return i
	}
	if toks[i+1] != "on" {
		name := toks[i+1]
		f, ok := w.fragments[name]
		if !ok {
			w.fail("no fragment named %s is defined", name)
			return i + 1
		}
		w.checkSpreadTarget(parent, typeName, f.on)
		w.walkSelection(f.body, f.on)
		return i + 1
	}
	if i+3 >= len(toks) || toks[i+3] != "{" {
		w.fail("%s: an inline fragment with no selection", typeName)
		return i + 1
	}
	on := toks[i+2]
	w.checkSpreadTarget(parent, typeName, on)
	end := matchBrace(toks, i+3)
	w.walkSelection(toks[i+4:end], on)
	return end
}

// checkSpreadTarget rejects a fragment on a type the parent can never be. On
// a union that is the only thing keeping the inline fragments honest.
func (w *walker) checkSpreadTarget(parent schemaType, typeName, on string) {
	if on == typeName {
		return
	}
	if parent.Kind != "UNION" && parent.Kind != "INTERFACE" {
		w.fail("a fragment on %s cannot be spread into %s", on, typeName)
		return
	}
	for _, p := range parent.PossibleTypes {
		if p == on {
			return
		}
	}
	w.fail("%s is not one of the types %s can be", on, typeName)
}

// matchBrace returns the index of the "}" closing the "{" at open.
func matchBrace(toks []string, open int) int {
	depth := 0
	for i := open; i < len(toks); i++ {
		switch toks[i] {
		case "{":
			depth++
		case "}":
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(toks)
}

// tokenize keeps only what the walk needs: names, braces, colons and "...".
// Arguments carry no selections, so a balanced "(...)" is dropped whole --
// that also disposes of the variable definitions on the operation and of the
// input objects the mutations pass.
func tokenize(doc string) []string {
	var toks []string
	r := []rune(doc)
	for i := 0; i < len(r); i++ {
		c := r[i]
		switch {
		case c == '#':
			for i < len(r) && r[i] != '\n' {
				i++
			}
		case c == '(':
			i = skipParens(r, i)
		case c == '{' || c == '}' || c == ':':
			toks = append(toks, string(c))
		case strings.HasPrefix(string(r[i:]), "..."):
			toks = append(toks, "...")
			i += 2
		case isNameStart(c):
			j := i
			for j < len(r) && isNameRune(r[j]) {
				j++
			}
			toks = append(toks, string(r[i:j]))
			i = j - 1
		}
	}
	return toks
}

func skipParens(r []rune, i int) int {
	depth := 0
	for ; i < len(r); i++ {
		switch r[i] {
		case '"':
			for i++; i < len(r) && r[i] != '"'; i++ {
				if r[i] == '\\' {
					i++
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return i
}

func isNameStart(c rune) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameRune(c rune) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}
