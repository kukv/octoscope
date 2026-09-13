package gql

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

// rollupPage wraps contexts nodes in the shape the query selects them in.
func rollupPage(pageInfo, nodes string) string {
	return `{"data":{"repository":{"pullRequest":{"commits":{"nodes":[{"commit":` +
		`{"statusCheckRollup":{"contexts":{"pageInfo":` + pageInfo + `,"nodes":[` + nodes + `]}}}}]}}}}}`
}

// fakeSeq answers each call with the next of a fixed sequence of bodies.
type fakeSeq struct {
	outs  []string
	calls [][]Var
}

func (f *fakeSeq) do(_ context.Context, _ string, vars []Var) ([]byte, error) {
	f.calls = append(f.calls, vars)
	i := len(f.calls) - 1
	if i >= len(f.outs) {
		return nil, fmt.Errorf("unexpected call %d: %v", i, vars)
	}
	return []byte(f.outs[i]), nil
}

func TestPRChecksWalksEveryPageOfContexts(t *testing.T) {
	t.Parallel()

	page1 := rollupPage(`{"hasNextPage":true,"endCursor":"CUR1"}`,
		`{"__typename":"CheckRun","name":"lint","status":"COMPLETED","conclusion":"SUCCESS"}`)
	page2 := rollupPage(`{"hasNextPage":false,"endCursor":"CUR2"}`,
		`{"__typename":"CheckRun","name":"test","status":"COMPLETED","conclusion":"FAILURE"}`)

	f := &fakeSeq{outs: []string{page1, page2}}
	c := &Client{Do: f.do}

	runs, err := c.PRChecks(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	if len(f.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (the first page says there is another)", len(f.calls))
	}
	// Without the cursor the second request asks for page one again and the
	// loop never ends.
	if !slices.Contains(f.calls[1], S("after", "CUR1")) {
		t.Errorf("second call = %v, want it to carry after=CUR1", f.calls[1])
	}
	if len(runs) != 2 {
		t.Errorf("runs = %d, want 2 (one from each page)", len(runs))
	}
}

func TestPRChecksReadsTheIdsTheViewActsOn(t *testing.T) {
	t.Parallel()

	c := fileClient(t, "testdata/pr_checks.json")
	runs, err := c.PRChecks(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	i := slices.IndexFunc(runs, func(r CheckRun) bool { return r.Name == "lint" })
	if i < 0 {
		t.Fatalf("no check named lint in %v", runs)
	}
	got := runs[i]
	if got.Typename != "CheckRun" {
		t.Errorf("Typename = %q, want CheckRun", got.Typename)
	}
	if got.DatabaseID != 101635448466 {
		t.Errorf("DatabaseID = %d, want 101635448466", got.DatabaseID)
	}
	if got.CheckSuite.WorkflowRun == nil || got.CheckSuite.WorkflowRun.DatabaseID != 34087925535 {
		t.Errorf("CheckSuite.WorkflowRun.DatabaseID = %v, want 34087925535", got.CheckSuite.WorkflowRun)
	}
	if got.CheckSuite.WorkflowRun == nil || got.CheckSuite.WorkflowRun.Workflow.Name != "CI" {
		t.Errorf("CheckSuite.WorkflowRun.Workflow.Name = %v, want %q", got.CheckSuite.WorkflowRun, "CI")
	}
	if got.StartedAt.IsZero() || got.CompletedAt.IsZero() || !got.CompletedAt.After(got.StartedAt) {
		t.Errorf("StartedAt/CompletedAt = %v/%v, want CompletedAt after StartedAt", got.StartedAt, got.CompletedAt)
	}
}
