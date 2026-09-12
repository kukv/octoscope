package gql

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
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

	checks, err := c.PRChecks(context.Background(), "kukv/octoscope", 61)
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
	if checks.Total != 2 {
		t.Errorf("Total = %d, want 2 (one from each page)", checks.Total)
	}
}

func TestPRChecksReadsTheIdsTheViewActsOn(t *testing.T) {
	t.Parallel()

	c := fileClient(t, "testdata/pr_checks.json")
	checks, err := c.PRChecks(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	i := slices.IndexFunc(checks.Runs, func(r gh.CheckRun) bool { return r.Name == "lint" })
	if i < 0 {
		t.Fatalf("no check named lint in %v", checks.Runs)
	}
	got := checks.Runs[i]
	if got.Kind != gh.CheckKindRun {
		t.Errorf("Kind = %v, want CheckKindRun", got.Kind)
	}
	if got.JobID != 101635448466 {
		t.Errorf("JobID = %d, want 101635448466", got.JobID)
	}
	if got.RunID != 34087925535 {
		t.Errorf("RunID = %d, want 34087925535", got.RunID)
	}
	if got.Workflow != "CI" {
		t.Errorf("Workflow = %q, want %q", got.Workflow, "CI")
	}
	if got.Duration() == 0 {
		t.Error("Duration() = 0, want the time between startedAt and completedAt")
	}
}
