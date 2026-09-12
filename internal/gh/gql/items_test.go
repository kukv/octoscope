package gql

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// fixedTransport answers every document with one recorded body.
func fixedTransport(t *testing.T, path string) (*Client, *[]Var) {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var got []Var
	c := &Client{Do: func(_ context.Context, _ string, vars []Var) ([]byte, error) {
		got = vars
		return body, nil
	}}
	return c, &got
}

func TestListPRsReadsTheRecordedAnswer(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_prs.json")
	prs, err := c.ListPRs(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	if len(prs) == 0 {
		t.Fatal("no pull requests decoded")
	}
	first := prs[0]
	if first.Number == 0 || first.Title == "" || first.URL == "" {
		t.Errorf("number/title/url not filled: %+v", first)
	}
	if first.State != gh.StateOpen {
		t.Errorf("state = %v, want open", first.State)
	}
	if first.Author.Login == "" {
		t.Error("author not filled")
	}
}

// The Repos tab shows the review decision and the check roll-up, and those
// two are exactly what REST could not answer. A document that stops selecting
// them decodes into a screen with empty columns and no error.
func TestListPRsFillsTheFieldsRESTCannotAnswer(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_prs.json")
	prs, err := c.ListPRs(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	var sawReview, sawChecks, sawSize bool
	for _, pr := range prs {
		if pr.Review != gh.ReviewNone {
			sawReview = true
		}
		if pr.Checks.Total > 0 {
			sawChecks = true
		}
		if pr.Additions > 0 || pr.Deletions > 0 {
			sawSize = true
		}
	}
	if !sawReview {
		t.Error("no review decision in the whole answer")
	}
	if !sawChecks {
		t.Error("no check roll-up in the whole answer")
	}
	if !sawSize {
		t.Error("no additions/deletions in the whole answer")
	}
}

// stripComments removes "#" comment lines from an embedded GraphQL document.
// The header comments repeat words like "first: 100" and "reviewDecision" in
// prose, so an assertion against the raw document text can pass on a comment
// instead of the selection it names.
func stripComments(doc string) string {
	var out strings.Builder
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// listDocuments names the connection each list document sends its
// page-size and ordering arguments to.
var listDocuments = map[string]struct {
	doc        string
	connection string
}{
	"repo_prs.graphql":    {repoPRsQuery, "pullRequests"},
	"repo_issues.graphql": {repoIssuesQuery, "issues"},
}

// topLevelConnectionArgs returns the argument list of one document's
// top-level connection (pullRequests(...) or issues(...)), comments already
// stripped. Page-size and ordering assertions have to be scoped to this
// block: the same argument text also appears, verbatim, in the labels(...)
// sub-connection every list document selects.
func topLevelConnectionArgs(t *testing.T, doc, connection string) string {
	t.Helper()

	clean := stripComments(doc)
	start := strings.Index(clean, connection+"(")
	if start == -1 {
		t.Fatalf("document has no %s( connection", connection)
	}
	start += len(connection)
	depth := 0
	for i := start; i < len(clean); i++ {
		switch clean[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return clean[start : i+1]
			}
		}
	}
	t.Fatalf("unbalanced parens in %s( connection", connection)
	return ""
}

// Nothing between the document and the screen reorders the list, so the
// document has to ask for the order gh asked for.
func TestTheListDocumentsAskForTheOrderGhAskedFor(t *testing.T) {
	t.Parallel()

	for name, d := range listDocuments {
		block := topLevelConnectionArgs(t, d.doc, d.connection)
		if !strings.Contains(block, "field: CREATED_AT") ||
			!strings.Contains(block, "direction: DESC") {
			t.Errorf("%s does not order by CREATED_AT DESC", name)
		}
	}
}

// gh pr list was asked for --limit 100; the documents have to ask for as
// many, or a busy repository silently loses rows.
func TestTheListDocumentsAskForAHundred(t *testing.T) {
	t.Parallel()

	for name, d := range listDocuments {
		block := topLevelConnectionArgs(t, d.doc, d.connection)
		if !strings.Contains(block, "first: 100") {
			t.Errorf("%s does not ask for 100 items", name)
		}
	}
}

// reviewDecision, statusCheckRollup, additions and deletions are exactly what
// REST cannot answer about a pull request; that gap is the reason this
// document exists instead of a REST call. A fixture-based test cannot catch
// one of these being dropped from the document (the fixture already has the
// data on disk, independent of what the document currently asks for), so
// this reads the embedded document text directly, comments stripped.
func TestRepoPRsDocumentSelectsTheFieldsRESTCannotAnswer(t *testing.T) {
	t.Parallel()

	doc := stripComments(repoPRsQuery)
	for _, field := range []string{"reviewDecision", "statusCheckRollup", "additions", "deletions"} {
		if !strings.Contains(doc, field) {
			t.Errorf("repo_prs.graphql does not select %s", field)
		}
	}
}

func TestListIssuesReadsTheRecordedAnswer(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_issues.json")
	issues, err := c.ListIssues(context.Background(), "kukv/octoscope")
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) == 0 {
		t.Fatal("no issues decoded")
	}
	if issues[0].Number == 0 || issues[0].Title == "" {
		t.Errorf("number/title not filled: %+v", issues[0])
	}
}

// The repository is named by two variables, not by one "owner/name" string:
// GraphQL's repository() takes the halves separately.
func TestTheListCallsNameTheRepositoryByItsTwoHalves(t *testing.T) {
	t.Parallel()

	c, got := fixedTransport(t, "testdata/repo_prs.json")
	if _, err := c.ListPRs(context.Background(), "cli/cli"); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	want := map[string]string{"owner": "cli", "name": "cli"}
	for _, v := range *got {
		if w, ok := want[v.Name]; ok && v.Str == w {
			delete(want, v.Name)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing variables: %v (got %+v)", want, *got)
	}
}

func TestListPRsRejectsARepositoryWithoutASeparator(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_prs.json")
	if _, err := c.ListPRs(context.Background(), "octoscope"); err == nil {
		t.Fatal("want an error for a repository with no owner/name separator")
	}
}
