package usecase

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
)

// fakeRepoFinder answers per owner, and records which owners were asked for.
type fakeRepoFinder struct {
	orgs    []string
	byOwner map[string][]gh.RepoCandidate
	owners  []string

	ownErr  error
	orgsErr error
	orgErr  map[string]error
}

func (f *fakeRepoFinder) SearchRepos(_ context.Context, query string, limit int) ([]gh.RepoCandidate, error) {
	return []gh.RepoCandidate{{Name: "charmbracelet/" + query, Stars: limit}}, nil
}

func (f *fakeRepoFinder) ListOwnRepos(_ context.Context, owner string, _ int) ([]gh.RepoCandidate, error) {
	f.owners = append(f.owners, owner)
	if owner == "" && f.ownErr != nil {
		return nil, f.ownErr
	}
	if err := f.orgErr[owner]; err != nil {
		return nil, err
	}
	return f.byOwner[owner], nil
}

func (f *fakeRepoFinder) ListOrgs(context.Context) ([]string, error) {
	return f.orgs, f.orgsErr
}

type fakeRepoStore struct {
	saved []string
	err   error
}

func (f *fakeRepoStore) SaveRepositories(repos []string) error {
	f.saved = repos
	return f.err
}

func names(candidates []gh.RepoCandidate) []string {
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.Name
	}
	return out
}

// Seeding is the one operation here that no single call answers: the user's
// own repositories and each organisation's are separate requests, and
// leaving any of them out would offer a first run half a list.
func TestSeedCandidatesCoversTheUserAndEveryOrg(t *testing.T) {
	t.Parallel()

	f := &fakeRepoFinder{
		orgs: []string{"charmbracelet", "kukv-org"},
		byOwner: map[string][]gh.RepoCandidate{
			"":              {{Name: "kukv/octoscope"}},
			"charmbracelet": {{Name: "charmbracelet/lipgloss"}},
			"kukv-org":      {{Name: "kukv-org/thing"}},
		},
	}
	got, err := (&Usecase{repos: f}).SeedCandidates(t.Context())
	if err != nil {
		t.Fatalf("SeedCandidates: %v", err)
	}
	for _, want := range []string{"kukv/octoscope", "charmbracelet/lipgloss", "kukv-org/thing"} {
		if !slices.Contains(names(got), want) {
			t.Errorf("candidates %v are missing %s", names(got), want)
		}
	}
}

// A token without the organisation scope is common, and it must not cost the
// user the repositories that were fetched before it.
func TestSeedCandidatesStillAnswersWhenTheOrgsAreUnreadable(t *testing.T) {
	t.Parallel()

	f := &fakeRepoFinder{
		orgsErr: errors.New("HTTP 403"),
		byOwner: map[string][]gh.RepoCandidate{"": {{Name: "kukv/octoscope"}}},
	}
	got, err := (&Usecase{repos: f}).SeedCandidates(t.Context())
	if err != nil {
		t.Fatalf("SeedCandidates: %v", err)
	}
	if !slices.Equal(names(got), []string{"kukv/octoscope"}) {
		t.Errorf("candidates = %v, want the user's own repositories", names(got))
	}
}

// One organisation the user cannot read must not take the others with it.
func TestSeedCandidatesSkipsOnlyTheOrgThatFailed(t *testing.T) {
	t.Parallel()

	f := &fakeRepoFinder{
		orgs:   []string{"closed-org", "kukv-org"},
		orgErr: map[string]error{"closed-org": errors.New("HTTP 403")},
		byOwner: map[string][]gh.RepoCandidate{
			"":         {{Name: "kukv/octoscope"}},
			"kukv-org": {{Name: "kukv-org/thing"}},
		},
	}
	got, err := (&Usecase{repos: f}).SeedCandidates(t.Context())
	if err != nil {
		t.Fatalf("SeedCandidates: %v", err)
	}
	if !slices.Contains(names(got), "kukv-org/thing") {
		t.Errorf("candidates = %v, want the readable organisation kept", names(got))
	}
}

// Without the user's own repositories there is nothing worth offering, so
// that failure is the one that travels back.
func TestSeedCandidatesFailsWhenTheUsersOwnRepositoriesAreUnreadable(t *testing.T) {
	t.Parallel()

	f := &fakeRepoFinder{ownErr: errors.New("HTTP 401")}
	if _, err := (&Usecase{repos: f}).SeedCandidates(t.Context()); err == nil {
		t.Error("SeedCandidates reported success with nothing to offer")
	}
}

// A repository the user owns inside an organisation comes back from both
// calls, and offering it twice would make the dialog look broken.
func TestSeedCandidatesDropsDuplicates(t *testing.T) {
	t.Parallel()

	f := &fakeRepoFinder{
		orgs: []string{"kukv-org"},
		byOwner: map[string][]gh.RepoCandidate{
			"":         {{Name: "kukv-org/thing"}},
			"kukv-org": {{Name: "kukv-org/thing"}},
		},
	}
	got, err := (&Usecase{repos: f}).SeedCandidates(t.Context())
	if err != nil {
		t.Fatalf("SeedCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("candidates = %v, want one", names(got))
	}
}

func TestSaveRepositoriesReachesTheStore(t *testing.T) {
	t.Parallel()

	store := &fakeRepoStore{}
	if err := (&Usecase{repoStore: store}).SaveRepositories([]string{"kukv/koto"}); err != nil {
		t.Fatalf("SaveRepositories: %v", err)
	}
	if !slices.Equal(store.saved, []string{"kukv/koto"}) {
		t.Errorf("store holds %v, want [kukv/koto]", store.saved)
	}
}
