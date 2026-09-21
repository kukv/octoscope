package gh

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// fakeChecksFetcher answers PRChecks, JobLog and RerunWorkflow with whatever
// a test sets. Embedding the nil backend panics loudly if a test calls a
// method it did not mean to exercise.
type fakeChecksFetcher struct {
	backend
	prChecks      func(ctx context.Context, repo string, number int) ([]gql.CheckRun, error)
	jobLog        func(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]github.LogLine, error)
	rerunWorkflow func(ctx context.Context, repo string, runID int64, scope github.RerunScope) error
}

func (f fakeChecksFetcher) PRChecks(ctx context.Context, repo string, number int) ([]gql.CheckRun, error) {
	return f.prChecks(ctx, repo, number)
}

func (f fakeChecksFetcher) JobLog(ctx context.Context, repo string, jobID int64, failedOnly bool) ([]github.LogLine, error) {
	return f.jobLog(ctx, repo, jobID, failedOnly)
}

func (f fakeChecksFetcher) RerunWorkflow(ctx context.Context, repo string, runID int64, scope github.RerunScope) error {
	return f.rerunWorkflow(ctx, repo, runID, scope)
}

// TestToCheckRunTranslatesACheckRun gives every field a CheckRun node can
// carry a distinct, non-zero value. Context and State belong to the
// StatusContext shape and TargetURL/CreatedAt likewise: GitHub's union type
// never sends them alongside a CheckRun's own fields, so they stay at their
// zero value here and are covered instead by
// TestToCheckRunTranslatesAStatusContext below.
func TestToCheckRunTranslatesACheckRun(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 9, 7, 5, 44, 1, 0, time.UTC)
	completedAt := time.Date(2026, 9, 7, 5, 44, 37, 0, time.UTC)

	n := gql.CheckRun{
		CheckContext: gql.CheckContext{
			Typename:   "CheckRun",
			Name:       "lint",
			Status:     "COMPLETED",
			Conclusion: "SUCCESS",
		},
		DatabaseID:  101635448466,
		DetailsURL:  "https://github.com/kukv/octoscope/actions/runs/34087925535/job/101635448466",
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	}
	n.CheckSuite.WorkflowRun = &gql.WorkflowRun{
		DatabaseID: 34087925535,
		RunNumber:  88,
	}
	n.CheckSuite.WorkflowRun.Workflow.Name = "CI"

	got := toCheckRun(n)
	want := domain.CheckRun{
		Name:        "lint",
		State:       domain.CheckSuccess,
		Kind:        domain.CheckKindRun,
		Workflow:    "CI",
		RunNumber:   88,
		Job:         domain.JobHandle("101635448466"),
		WorkflowRun: domain.RunHandle("34087925535"),
		URL:         "https://github.com/kukv/octoscope/actions/runs/34087925535/job/101635448466",
		StartedAt:   startedAt,
		CompletedAt: completedAt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toCheckRun() = %+v, want %+v", got, want)
	}
}

// TestToCheckRunTranslatesAStatusContext covers the fields a CheckRun-shaped
// fixture cannot: Context, State, TargetURL and CreatedAt. Workflow,
// RunNumber, Job, WorkflowRun and CompletedAt stay zero: a StatusContext has no
// job, no run and no completion time of its own.
func TestToCheckRunTranslatesAStatusContext(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 9, 7, 5, 44, 1, 0, time.UTC)

	n := gql.CheckRun{
		CheckContext: gql.CheckContext{
			Typename: "StatusContext",
			Context:  "ci/deploy",
			State:    "FAILURE",
		},
		TargetURL: "https://ci.example.com/deploy/42",
		CreatedAt: createdAt,
	}

	got := toCheckRun(n)
	want := domain.CheckRun{
		Name:      "ci/deploy",
		State:     domain.CheckFailure,
		Kind:      domain.CheckKindStatus,
		URL:       "https://ci.example.com/deploy/42",
		StartedAt: createdAt,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toCheckRun() = %+v, want %+v", got, want)
	}
}

// TestToChecksCountsAndRollsUpEachRun covers the computation toChecks does
// on top of toCheckRun: Total, Passed, Failed and Running are counted, not
// copied, and State is the rollup's own worst-of-all-runs reading.
func TestToChecksCountsAndRollsUpEachRun(t *testing.T) {
	t.Parallel()

	runs := []gql.CheckRun{
		{CheckContext: gql.CheckContext{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"}},
		{CheckContext: gql.CheckContext{Typename: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"}},
		{CheckContext: gql.CheckContext{Typename: "CheckRun", Name: "deploy", Status: "IN_PROGRESS"}},
		{CheckContext: gql.CheckContext{Typename: "StatusContext", Context: "ci/legacy", State: "SUCCESS"}},
	}

	got := toChecks(runs)
	want := domain.Checks{
		Total:   4,
		Passed:  2,
		Failed:  1,
		Running: 1,
		State:   domain.CheckFailure,
		Runs: []domain.CheckRun{
			{Name: "build", State: domain.CheckSuccess, Kind: domain.CheckKindRun},
			{Name: "test", State: domain.CheckFailure, Kind: domain.CheckKindRun},
			{Name: "deploy", State: domain.CheckRunning, Kind: domain.CheckKindRun},
			{Name: "ci/legacy", State: domain.CheckSuccess, Kind: domain.CheckKindStatus},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toChecks() = %+v, want %+v", got, want)
	}
}

// TestToCheckStateCoversEveryOutcome exhausts both rollup shapes' status
// vocabulary: a CheckRun reports status/conclusion, a StatusContext reports
// a single state, and the two never overlap in what GitHub actually sends.
func TestToCheckStateCoversEveryOutcome(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		node gql.CheckContext
		want domain.CheckState
	}{
		{"CheckRun success", gql.CheckContext{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "SUCCESS"}, domain.CheckSuccess},
		{"CheckRun failure", gql.CheckContext{Typename: "CheckRun", Status: "COMPLETED", Conclusion: "FAILURE"}, domain.CheckFailure},
		{"CheckRun in progress", gql.CheckContext{Typename: "CheckRun", Status: "IN_PROGRESS"}, domain.CheckRunning},
		{"StatusContext success", gql.CheckContext{Typename: "StatusContext", State: "SUCCESS"}, domain.CheckSuccess},
		{"StatusContext failure", gql.CheckContext{Typename: "StatusContext", State: "FAILURE"}, domain.CheckFailure},
		{"StatusContext error", gql.CheckContext{Typename: "StatusContext", State: "ERROR"}, domain.CheckFailure},
		{"StatusContext pending", gql.CheckContext{Typename: "StatusContext", State: "PENDING"}, domain.CheckPending},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := toCheckState(tt.node); got != tt.want {
				t.Errorf("toCheckState(%+v) = %v, want %v", tt.node, got, tt.want)
			}
		})
	}
}

// TestToChecksFromContextsRollsUpTheRawRollup covers the conversion a search
// or item document's embedded rollup goes through: unlike toChecks, it
// starts from the plainer gql.CheckContext shape a search or single-item
// document selects, and rolls up the same way.
func TestToChecksFromContextsRollsUpTheRawRollup(t *testing.T) {
	t.Parallel()

	contexts := []gql.CheckContext{
		{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"},
		{Typename: "CheckRun", Name: "test", Status: "COMPLETED", Conclusion: "FAILURE"},
		{Typename: "CheckRun", Name: "deploy", Status: "IN_PROGRESS"},
		{Typename: "StatusContext", Context: "ci/legacy", State: "SUCCESS"},
	}

	got := toChecksFromContexts(contexts)
	want := domain.Checks{
		Total:   4,
		Passed:  2,
		Failed:  1,
		Running: 1,
		State:   domain.CheckFailure,
		Runs: []domain.CheckRun{
			{Name: "build", State: domain.CheckSuccess, Kind: domain.CheckKindRun},
			{Name: "test", State: domain.CheckFailure, Kind: domain.CheckKindRun},
			{Name: "deploy", State: domain.CheckRunning, Kind: domain.CheckKindRun},
			{Name: "ci/legacy", State: domain.CheckSuccess, Kind: domain.CheckKindStatus},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toChecksFromContexts() = %+v, want %+v", got, want)
	}
}

// TestPRChecksRollsUpWhatTheBackendReturns wires PRChecks through the
// gateway end to end: the backend answers with wire nodes, and the gateway
// hands back the rolled-up domain.Checks.
func TestPRChecksRollsUpWhatTheBackendReturns(t *testing.T) {
	t.Parallel()

	g := New(fakeChecksFetcher{prChecks: func(context.Context, string, int) ([]gql.CheckRun, error) {
		return []gql.CheckRun{
			{CheckContext: gql.CheckContext{Typename: "CheckRun", Name: "build", Status: "COMPLETED", Conclusion: "SUCCESS"}},
		}, nil
	}})

	got, err := g.PRChecks(context.Background(), "kukv/octoscope", 61)
	if err != nil {
		t.Fatalf("PRChecks: %v", err)
	}
	want := domain.Checks{
		Total:  1,
		Passed: 1,
		State:  domain.CheckSuccess,
		Runs:   []domain.CheckRun{{Name: "build", State: domain.CheckSuccess, Kind: domain.CheckKindRun}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PRChecks() = %+v, want %+v", got, want)
	}
}

// TestToLogLineTranslatesEveryField gives github.LogLine's three fields
// distinct, non-zero values and compares the whole resulting domain.LogLine.
func TestToLogLineTranslatesEveryField(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 7, 9, 15, 22, 123456700, time.UTC)
	got := toLogLine(github.LogLine{Step: "Run tests", Time: at, Text: "ok"})
	want := domain.LogLine{Step: "Run tests", Time: at, Text: "ok"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toLogLine() = %+v, want %+v", got, want)
	}
}

func TestJobLogTranslatesEveryLine(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 9, 7, 9, 15, 22, 0, time.UTC)
	g := New(fakeChecksFetcher{jobLog: func(context.Context, string, int64, bool) ([]github.LogLine, error) {
		return []github.LogLine{{Step: "Run tests", Time: at, Text: "ok"}}, nil
	}})

	got, err := g.JobLog(context.Background(), "kukv/octoscope", "42", false)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	want := []domain.LogLine{{Step: "Run tests", Time: at, Text: "ok"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("JobLog() = %+v, want %+v", got, want)
	}
}

// TestJobLogRejectsAHandleThatIsNotAnActionsID guards the parse: a handle
// that did not come out of this gateway must not reach the backend as if it
// were a valid Actions job id.
func TestJobLogRejectsAHandleThatIsNotAnActionsID(t *testing.T) {
	t.Parallel()

	g := New(fakeChecksFetcher{})
	if _, err := g.JobLog(context.Background(), "kukv/octoscope", domain.JobHandle("nope"), false); err == nil {
		t.Fatal("JobLog accepted a non-numeric handle")
	}
}

// TestPRChecksCarriesTheJobAndRunHandlesApart guards against the job and run
// handles being swapped or merged: they come from different DatabaseID
// fields of the same node.
func TestPRChecksCarriesTheJobAndRunHandlesApart(t *testing.T) {
	t.Parallel()

	n := gql.CheckRun{
		CheckContext: gql.CheckContext{Typename: "CheckRun", Name: "lint", Status: "COMPLETED", Conclusion: "SUCCESS"},
		DatabaseID:   11,
	}
	n.CheckSuite.WorkflowRun = &gql.WorkflowRun{DatabaseID: 22}

	g := New(fakeChecksFetcher{prChecks: func(context.Context, string, int) ([]gql.CheckRun, error) {
		return []gql.CheckRun{n}, nil
	}})

	c, err := g.PRChecks(context.Background(), "kukv/octoscope", 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.Runs[0].Job != domain.JobHandle("11") {
		t.Errorf("Job = %q, want %q", c.Runs[0].Job, "11")
	}
	if c.Runs[0].WorkflowRun != domain.RunHandle("22") {
		t.Errorf("WorkflowRun = %q, want %q", c.Runs[0].WorkflowRun, "22")
	}
}

// TestFromRerunScopeCoversEveryValue guards both directions of the enum:
// RerunScope is an argument, so a value the conversion does not recognise
// would silently send the wrong endpoint or flag rather than fail to compile.
func TestFromRerunScopeCoversEveryValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		scope domain.RerunScope
		want  github.RerunScope
	}{
		{domain.RerunFailed, github.RerunFailed},
		{domain.RerunAll, github.RerunAll},
	}
	for _, tt := range tests {
		if got := fromRerunScope(tt.scope); got != tt.want {
			t.Errorf("fromRerunScope(%v) = %v, want %v", tt.scope, got, tt.want)
		}
	}
}

func TestRerunWorkflowPassesTheConvertedScope(t *testing.T) {
	t.Parallel()

	var got github.RerunScope
	g := New(fakeChecksFetcher{rerunWorkflow: func(_ context.Context, _ string, _ int64, scope github.RerunScope) error {
		got = scope
		return nil
	}})

	if err := g.RerunWorkflow(context.Background(), "kukv/octoscope", "34087925535", domain.RerunAll); err != nil {
		t.Fatalf("RerunWorkflow: %v", err)
	}
	if got != github.RerunAll {
		t.Errorf("scope passed to the backend = %v, want RerunAll", got)
	}
}
