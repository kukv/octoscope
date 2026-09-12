package gql

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// commentsOnly is the answer pr_comments.graphql and issue_comments.graphql
// come back with: the connection and nothing else.
func commentsOnly(item, pageInfo, nodes string) string {
	return `{"data":{"repository":{"` + item + `":{"comments":{"pageInfo":` + pageInfo +
		`,"nodes":[` + nodes + `]}}}}}`
}

func commentNodeJSON(login string) string {
	return `{"author":{"login":"` + login + `"},"body":"a comment","createdAt":"2026-09-13T00:00:00Z"}`
}

// GitHub answers comments(first: 100) with the oldest hundred, so a detail
// view that takes one page drops the newest comments of a long thread and
// presents the rest as the whole conversation. gh pages the thread to the
// end, and so must this.
//
// The thread is three pages on purpose. Two pages cannot tell a walk that
// keeps going from one that fetches a single extra page and stops: both
// answer a two-page thread correctly, and the second is the same silent
// truncation one keyword further out.
func TestGetPRWalksEveryPageOfTheConversation(t *testing.T) {
	t.Parallel()

	page1 := `{"data":{"repository":{"pullRequest":{"number":59,"comments":{"pageInfo":` +
		`{"hasNextPage":true,"endCursor":"CUR1"},"nodes":[` + commentNodeJSON("oldest") + `]}}}}}`
	page2 := commentsOnly("pullRequest", `{"hasNextPage":true,"endCursor":"CUR2"}`, commentNodeJSON("middle"))
	page3 := commentsOnly("pullRequest", `{"hasNextPage":false,"endCursor":"CUR3"}`, commentNodeJSON("newest"))

	f := &fakeSeq{outs: []string{page1, page2, page3}}
	c := &Client{Do: f.do}

	pr, err := c.GetPR(context.Background(), "kukv/octoscope", 59)
	if err != nil {
		t.Fatalf("GetPR: %v", err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("calls = %d, want 3: the walk stops only when a page says there is no next one", len(f.calls))
	}
	// Each request carries the cursor the page before it ended on. Without
	// that, every later request asks for page one again.
	for i, want := range []string{"CUR1", "CUR2"} {
		if !slices.Contains(f.calls[i+1], S("after", want)) {
			t.Errorf("call %d = %v, want it to carry after=%s", i+1, f.calls[i+1], want)
		}
	}
	// The later pages are appended, not substituted, and the thread keeps the
	// order GitHub answered it in.
	if got := logins(pr.Comments); !slices.Equal(got, []string{"oldest", "middle", "newest"}) {
		t.Errorf("conversation = %v, want [oldest middle newest]", got)
	}
}

func TestGetIssueWalksEveryPageOfTheConversation(t *testing.T) {
	t.Parallel()

	page1 := `{"data":{"repository":{"issue":{"number":54,"comments":{"pageInfo":` +
		`{"hasNextPage":true,"endCursor":"CUR1"},"nodes":[` + commentNodeJSON("oldest") + `]}}}}}`
	page2 := commentsOnly("issue", `{"hasNextPage":false,"endCursor":"CUR2"}`, commentNodeJSON("newest"))

	f := &fakeSeq{outs: []string{page1, page2}}
	c := &Client{Do: f.do}

	issue, err := c.GetIssue(context.Background(), "kukv/octoscope", 54)
	if err != nil {
		t.Fatalf("GetIssue: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (the first page says there is another)", len(f.calls))
	}
	if !slices.Contains(f.calls[1], S("after", "CUR1")) {
		t.Errorf("second call = %v, want it to carry after=CUR1", f.calls[1])
	}
	if got := logins(issue.Comments); !slices.Equal(got, []string{"oldest", "newest"}) {
		t.Errorf("conversation = %v, want [oldest newest]", got)
	}
}

// A fake answers with whatever pageInfo the test wrote, so only the document
// text says whether the real answer will carry one. Without it every thread
// looks like it ends at the first page and the walk never starts.
func TestTheSingleItemDocumentsAskWhetherTheConversationGoesOn(t *testing.T) {
	t.Parallel()

	docs := map[string]string{"pr.graphql": prQuery, "issue.graphql": issueQuery}
	for name, doc := range docs {
		clean := stripComments(doc)
		if !strings.Contains(clean, "hasNextPage") || !strings.Contains(clean, "endCursor") {
			t.Errorf("%s does not read the comments' pageInfo", name)
		}
	}
}

// The paging documents ask only for the comments: re-requesting the whole
// item on every page would fetch its labels, assignees and check roll-up
// again for each hundred comments.
func TestTheCommentPagingDocumentsSelectNothingButTheComments(t *testing.T) {
	t.Parallel()

	docs := map[string]string{"pr_comments.graphql": prCommentsQuery, "issue_comments.graphql": issueCommentsQuery}
	for name, doc := range docs {
		clean := stripComments(doc)
		if !strings.Contains(clean, "after: $after") {
			t.Errorf("%s does not take a cursor, so it can only ask for page one", name)
		}
		if !strings.Contains(clean, "hasNextPage") {
			t.Errorf("%s does not read pageInfo, so the walk stops after one extra page", name)
		}
		for _, unwanted := range []string{"labels", "assignees", "commits", "title"} {
			if strings.Contains(clean, unwanted) {
				t.Errorf("%s selects %s, which the first page already carried", name, unwanted)
			}
		}
	}
}

// logins names the authors of a conversation in order.
func logins(comments []gh.Comment) []string {
	out := make([]string, len(comments))
	for i, c := range comments {
		out[i] = c.Author.Login
	}
	return out
}
