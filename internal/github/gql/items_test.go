package gql

import (
	"context"
	"errors"
	"os"
	"regexp"
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
	for _, pr := range prs {
		// The board sorts on this one, and a zero time sorts every row to
		// the same place without any error to notice.
		if pr.UpdatedAt.IsZero() {
			t.Errorf("pr #%d has no updatedAt", pr.Number)
		}
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
	// The Work and Repos tabs draw the author, open the URL with o, and sort
	// on updatedAt. Each of the three is a column that goes blank, or a row
	// that sorts to the wrong place, with no error to notice.
	if issues[0].Author.Login == "" {
		t.Error("author not filled")
	}
	if issues[0].URL == "" {
		t.Error("url not filled; o has nothing to open")
	}
	if issues[0].UpdatedAt.IsZero() {
		t.Error("updatedAt not filled")
	}
}

// The repository is named by two variables, not by one "owner/name" string:
// GraphQL's repository() takes the halves separately. The two halves of the
// test repository differ, so a swapped pair does not read as a pass.
func TestTheListCallsNameTheRepositoryByItsTwoHalves(t *testing.T) {
	t.Parallel()

	c, got := fixedTransport(t, "testdata/repo_prs.json")
	if _, err := c.ListPRs(context.Background(), "kukv/octoscope"); err != nil {
		t.Fatalf("ListPRs: %v", err)
	}
	want := map[string]string{"owner": "kukv", "name": "octoscope"}
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

// wordBody matches the field name "body" and nothing else -- PullRequest,
// Issue and IssueComment all also have a real "bodyHTML" field, and a plain
// substring match would count one of those as the item's own body.
var wordBody = regexp.MustCompile(`\bbody\b`)

// A fixture-based test cannot notice body or comments being dropped from the
// single-item documents: the recorded fixture already has that data on disk
// no matter what the document currently asks for. This reads the embedded
// document text instead, comments stripped. "body" has to be counted rather
// than just matched: both documents also select a "body" on each comment
// node, so dropping the item's own body would still leave the word present.
func TestSingleItemDocumentsSelectTheBodyAndTheConversation(t *testing.T) {
	t.Parallel()

	docs := map[string]string{"pr.graphql": prQuery, "issue.graphql": issueQuery}
	for name, doc := range docs {
		clean := stripComments(doc)
		if n := len(wordBody.FindAllString(clean, -1)); n < 2 {
			t.Errorf("%s selects body %d times, want at least 2 (the item's own and each comment's)", name, n)
		}
		if !strings.Contains(clean, "comments(first: 100)") {
			t.Errorf("%s does not select comments", name)
		}
	}
}

func TestGetPRFillsTheBodyAndTheConversation(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/pr.json")
	pr, err := c.GetPR(context.Background(), "kukv/octoscope", 59)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if pr.Number == 0 || pr.Title == "" {
		t.Errorf("number/title not filled: %+v", pr)
	}
	if pr.Body == "" {
		t.Error("body not filled; the list document leaves it empty, the single one must not")
	}
	if len(pr.Comments) == 0 {
		t.Fatal("no comments decoded")
	}
	if pr.Comments[0].Author.Login == "" || pr.Comments[0].Body == "" {
		t.Errorf("comment not filled: %+v", pr.Comments[0])
	}
	if pr.Comments[0].CreatedAt.IsZero() {
		t.Error("comment has no timestamp")
	}
	// The detail view names the branches the pull request merges between,
	// and the size of the diff. Both pairs are two fields of the same type
	// side by side, so only the recorded values say they were not swapped.
	if pr.Head != "worktree-eventual-singing-gem" || pr.Base != "main" {
		t.Errorf("head/base = %q/%q, want worktree-eventual-singing-gem/main", pr.Head, pr.Base)
	}
	if pr.Additions != 6322 || pr.Deletions != 486 {
		t.Errorf("additions/deletions = %d/%d, want 6322/486", pr.Additions, pr.Deletions)
	}
	// GraphQL nests both of these under a "nodes" array, so a tag that names
	// the connection instead of its nodes decodes into an empty list and the
	// detail view shows an item with no labels and nobody assigned.
	if len(pr.Assignees) == 0 {
		t.Error("no assignees decoded")
	} else if pr.Assignees[0].Login == "" {
		t.Errorf("assignee not filled: %+v", pr.Assignees[0])
	}
	if len(pr.Labels) == 0 {
		t.Error("no labels decoded")
	} else if pr.Labels[0].Name == "" || pr.Labels[0].Color == "" {
		t.Errorf("label not filled: %+v", pr.Labels[0])
	}
}

// The number is a GraphQL Int. A transport that spells it as a string gets
// the whole document rejected before any of it runs.
func TestGetPRSendsTheNumberAsANumber(t *testing.T) {
	t.Parallel()

	c, got := fixedTransport(t, "testdata/pr.json")
	if _, err := c.GetPR(context.Background(), "kukv/octoscope", 59); err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	for _, v := range *got {
		if v.Name == "number" {
			if v.Kind != VarInt {
				t.Errorf("number is %v, want VarInt", v.Kind)
			}
			if v.Int != 59 {
				t.Errorf("number = %d, want 59", v.Int)
			}
			return
		}
	}
	t.Errorf("no number variable in %+v", *got)
}

func TestRepoNameReadsTheCanonicalName(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/repo_name.json")
	name, err := c.RepoName(context.Background(), "kukv/octoscope")
	if err != nil {
		t.Fatalf("RepoName: %v", err)
	}
	if name != "kukv/octoscope" {
		t.Errorf("name = %q, want kukv/octoscope", name)
	}
}

// A repository nobody can see comes back as a null node beside an errors
// array. "" with no error would read as "this directory has no repository",
// which is a different thing from "GitHub refused".
func TestRepoNameReportsAFailureRatherThanAnEmptyName(t *testing.T) {
	t.Parallel()

	c := &Client{Do: func(context.Context, string, []Var) ([]byte, error) {
		return []byte(`{"data":{"repository":null}}`), errors.New("Could not resolve to a Repository")
	}}
	if _, err := c.RepoName(context.Background(), "kukv/nope"); err == nil {
		t.Fatal("want the transport's error back")
	}
}

// The Repos tab uses this answer to decide whether the working directory has
// a repository at all, and GitHub returns the current name, which is how a
// rename reaches the tab. A fixture-based test cannot notice nameWithOwner
// being dropped from the document: the recorded fixture already has it on
// disk no matter what the document currently asks for.
func TestRepoNameDocumentSelectsNameWithOwner(t *testing.T) {
	t.Parallel()

	if !strings.Contains(stripComments(repoNameQuery), "nameWithOwner") {
		t.Error("repo_name.graphql does not select nameWithOwner")
	}
}

func TestGetIssueFillsTheBodyAndTheConversation(t *testing.T) {
	t.Parallel()

	c, _ := fixedTransport(t, "testdata/issue.json")
	issue, err := c.GetIssue(context.Background(), "kukv/octoscope", 54)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if issue.Number == 0 || issue.Title == "" {
		t.Errorf("number/title not filled: %+v", issue)
	}
	if issue.Body == "" {
		t.Error("body not filled")
	}
	if len(issue.Comments) == 0 {
		t.Fatal("no comments decoded")
	}
	if issue.Comments[0].Author.Login == "" || issue.Comments[0].Body == "" {
		t.Errorf("comment not filled: %+v", issue.Comments[0])
	}
	if issue.Comments[0].CreatedAt.IsZero() {
		t.Error("comment has no timestamp")
	}
	if len(issue.Assignees) == 0 {
		t.Error("no assignees decoded")
	} else if issue.Assignees[0].Login == "" {
		t.Errorf("assignee not filled: %+v", issue.Assignees[0])
	}
	if len(issue.Labels) == 0 {
		t.Error("no labels decoded")
	} else if issue.Labels[0].Name == "" || issue.Labels[0].Color == "" {
		t.Errorf("label not filled: %+v", issue.Labels[0])
	}
}

// The detail view's picker shows who an item is assigned to, and these two
// documents are where that answer comes from. The decode tests above read a
// recorded answer, which carries its assignees no matter what the document
// currently asks for; this reads the document text instead.
func TestSingleItemDocumentsSelectTheAssignees(t *testing.T) {
	t.Parallel()

	docs := map[string]string{"pr.graphql": prQuery, "issue.graphql": issueQuery}
	for name, doc := range docs {
		if !strings.Contains(stripComments(doc), "assignees(first: 100)") {
			t.Errorf("%s does not select assignees", name)
		}
	}
}
