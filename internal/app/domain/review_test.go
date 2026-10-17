package domain

import (
	"reflect"
	"testing"
)

func reviewFixture() ReviewContext {
	return ReviewContext{
		PullRequest: "PR_1",
		Pending:     "PRR_1",
		Threads: []ReviewThread{
			{Path: "pending.go", Comments: []ThreadComment{{Body: "mine", Pending: true}}},
			{Path: "mixed.go", Comments: []ThreadComment{
				{Body: "published", Pending: false},
				{Body: "my reply", Pending: true},
			}},
			{Path: "published.go", Comments: []ThreadComment{{Body: "theirs", Pending: false}}},
		},
	}
}

// Discarding deletes the unsubmitted review and every comment in it, and
// nothing else: the diff view applies this in place of refetching, so a
// published comment that went with it would vanish from the screen while
// it is still on GitHub.
func TestDiscardedDropsOnlyThePendingComments(t *testing.T) {
	t.Parallel()
	before := reviewFixture()
	got := before.Discarded()

	want := ReviewContext{
		PullRequest: "PR_1",
		Threads: []ReviewThread{
			{Path: "mixed.go", Comments: []ThreadComment{{Body: "published", Pending: false}}},
			{Path: "published.go", Comments: []ThreadComment{{Body: "theirs", Pending: false}}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Discarded() = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(before, reviewFixture()) {
		t.Errorf("Discarded() changed the context it was called on: %+v", before)
	}
}

// Submitting publishes every pending comment as it is; the diff view applies
// this in place of refetching, and a comment left marked pending would read
// as one that was never sent.
func TestSubmittedPublishesThePendingComments(t *testing.T) {
	t.Parallel()
	before := reviewFixture()
	got := before.Submitted()

	want := ReviewContext{
		PullRequest: "PR_1",
		Threads: []ReviewThread{
			{Path: "pending.go", Comments: []ThreadComment{{Body: "mine", Pending: false}}},
			{Path: "mixed.go", Comments: []ThreadComment{
				{Body: "published", Pending: false},
				{Body: "my reply", Pending: false},
			}},
			{Path: "published.go", Comments: []ThreadComment{{Body: "theirs", Pending: false}}},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Submitted() = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(before, reviewFixture()) {
		t.Errorf("Submitted() changed the context it was called on: %+v", before)
	}
}
