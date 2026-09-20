package gh

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
)

func prRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemPR, Repo: "kukv/octoscope", Number: 7}
}

func issueRef() domain.ItemRef {
	return domain.ItemRef{Kind: domain.ItemIssue, Repo: "kukv/octoscope", Number: 9}
}

// TestAddCommentPicksTheCallByKind is the dispatch that used to live in the
// usecase layer. The fake leaves the other call unset, so reaching for it
// panics rather than passing quietly.
func TestAddCommentPicksTheCallByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull request", func(t *testing.T) {
		t.Parallel()
		var gotRepo, gotBody string
		var gotNumber int
		b := fakeBackend{addPRComment: func(repo string, number int, body string) error {
			gotRepo, gotNumber, gotBody = repo, number, body
			return nil
		}}
		if err := New(b).AddComment(context.Background(), prRef(), "hello"); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if gotRepo != "kukv/octoscope" || gotNumber != 7 || gotBody != "hello" {
			t.Errorf("backend saw %q %d %q", gotRepo, gotNumber, gotBody)
		}
	})
	t.Run("issue", func(t *testing.T) {
		t.Parallel()
		var gotRepo, gotBody string
		var gotNumber int
		b := fakeBackend{addIssueComment: func(repo string, number int, body string) error {
			gotRepo, gotNumber, gotBody = repo, number, body
			return nil
		}}
		if err := New(b).AddComment(context.Background(), issueRef(), "hello"); err != nil {
			t.Fatalf("AddComment: %v", err)
		}
		if gotRepo != "kukv/octoscope" || gotNumber != 9 || gotBody != "hello" {
			t.Errorf("backend saw %q %d %q", gotRepo, gotNumber, gotBody)
		}
	})
}

// TestSetStatePicksTheCallByKindAndDirection covers all four combinations:
// two kinds times closing and reopening.
func TestSetStatePicksTheCallByKindAndDirection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		ref     domain.ItemRef
		closing bool
		set     func(called *string) fakeBackend
		want    string
	}{
		{
			name: "close a pull request", ref: prRef(), closing: true, want: "ClosePR",
			set: func(called *string) fakeBackend {
				return fakeBackend{closePR: func(string, int) error { *called = "ClosePR"; return nil }}
			},
		},
		{
			name: "reopen a pull request", ref: prRef(), closing: false, want: "ReopenPR",
			set: func(called *string) fakeBackend {
				return fakeBackend{reopenPR: func(string, int) error { *called = "ReopenPR"; return nil }}
			},
		},
		{
			name: "close an issue", ref: issueRef(), closing: true, want: "CloseIssue",
			set: func(called *string) fakeBackend {
				return fakeBackend{closeIssue: func(string, int) error { *called = "CloseIssue"; return nil }}
			},
		},
		{
			name: "reopen an issue", ref: issueRef(), closing: false, want: "ReopenIssue",
			set: func(called *string) fakeBackend {
				return fakeBackend{reopenIssue: func(string, int) error { *called = "ReopenIssue"; return nil }}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var called string
			if err := New(tt.set(&called)).SetState(context.Background(), tt.ref, tt.closing); err != nil {
				t.Fatalf("SetState: %v", err)
			}
			if called != tt.want {
				t.Errorf("called %q, want %q", called, tt.want)
			}
		})
	}
}

// TestEditLabelsPicksTheCallByKind and TestEditAssigneesPicksTheCallByKind
// check the two remaining pairs, and that add and remove arrive in order.
func TestEditLabelsPicksTheCallByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull request", func(t *testing.T) {
		t.Parallel()
		var add, remove []string
		b := fakeBackend{editPRLabels: func(_ string, _ int, a, r []string) error {
			add, remove = a, r
			return nil
		}}
		if err := New(b).EditLabels(context.Background(), prRef(), []string{"bug"}, []string{"wip"}); err != nil {
			t.Fatalf("EditLabels: %v", err)
		}
		if !slices.Equal(add, []string{"bug"}) || !slices.Equal(remove, []string{"wip"}) {
			t.Errorf("add = %v, remove = %v", add, remove)
		}
	})
	t.Run("issue", func(t *testing.T) {
		t.Parallel()
		var called bool
		b := fakeBackend{editIssueLabels: func(string, int, []string, []string) error {
			called = true
			return nil
		}}
		if err := New(b).EditLabels(context.Background(), issueRef(), nil, nil); err != nil {
			t.Fatalf("EditLabels: %v", err)
		}
		if !called {
			t.Error("EditIssueLabels was not called")
		}
	})
}

func TestEditAssigneesPicksTheCallByKind(t *testing.T) {
	t.Parallel()
	t.Run("pull request", func(t *testing.T) {
		t.Parallel()
		var add, remove []string
		b := fakeBackend{editPRAssignees: func(_ string, _ int, a, r []string) error {
			add, remove = a, r
			return nil
		}}
		if err := New(b).EditAssignees(context.Background(), prRef(), []string{"kukv"}, nil); err != nil {
			t.Fatalf("EditAssignees: %v", err)
		}
		if !slices.Equal(add, []string{"kukv"}) || remove != nil {
			t.Errorf("add = %v, remove = %v", add, remove)
		}
	})
	t.Run("issue", func(t *testing.T) {
		t.Parallel()
		var called bool
		b := fakeBackend{editIssueAssignees: func(string, int, []string, []string) error {
			called = true
			return nil
		}}
		if err := New(b).EditAssignees(context.Background(), issueRef(), nil, nil); err != nil {
			t.Fatalf("EditAssignees: %v", err)
		}
		if !called {
			t.Error("EditIssueAssignees was not called")
		}
	})
}

// TestAddCommentKeepsWhatTheClientSaid checks the claim the wrap in writes.go
// rests on: a failure the domain has a sentinel for gains that sentinel and
// keeps its own text, so what reaches the screen is unchanged.
func TestAddCommentKeepsWhatTheClientSaid(t *testing.T) {
	t.Parallel()
	b := fakeBackend{addPRComment: func(string, int, string) error {
		return fmt.Errorf("gh: %w", github.ErrUnauthenticated)
	}}
	err := New(b).AddComment(context.Background(), prRef(), "hello")
	if err == nil {
		t.Fatal("AddComment returned no error")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("error does not carry the domain's sentinel: %v", err)
	}
	if got, want := err.Error(), "gh: "+github.ErrUnauthenticated.Error(); got != want {
		t.Errorf("Error() = %q, want the client's own text %q", got, want)
	}
}
