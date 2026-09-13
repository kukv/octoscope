package gh

import (
	"context"
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// wrap is the only place a client's sentinel becomes the domain's; every
// override in this package depends on it to keep the fatal-error screen
// working, so each sentinel it knows about is checked here, plus the case it
// must never touch: an error none of them name.
func TestWrapTranslatesEverySentinel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   error
		want error
	}{
		{"not installed", github.ErrNotInstalled, domain.ErrBackendUnavailable},
		{"unauthenticated", github.Classify(github.ErrUnauthenticated, "gh: Bad credentials"), domain.ErrUnauthenticated},
		{"transient", github.Classify(github.ErrTransient, "gh: HTTP 502"), domain.ErrTransient},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wrap(tt.in)
			if !errors.Is(got, tt.want) {
				t.Errorf("wrap(%v) = %v, want errors.Is(_, %v)", tt.in, got, tt.want)
			}
			if got.Error() != tt.in.Error() {
				t.Errorf("wrap rewrote the text:\n got %q\nwant %q", got.Error(), tt.in.Error())
			}
		})
	}
}

// A sentinel wrap does not recognise must reach the caller unchanged: this is
// the one seam every override's error crosses, and swallowing an unknown
// error here would hide it from all of them.
func TestWrapLeavesAnUnknownErrorUnchanged(t *testing.T) {
	t.Parallel()

	in := errors.New("gh pr list: no pull requests match")
	got := wrap(in)
	if got != in {
		t.Errorf("wrap(%v) = %v, want the same error unchanged", in, got)
	}
	if errors.Is(got, domain.ErrBackendUnavailable) || errors.Is(got, domain.ErrUnauthenticated) {
		t.Errorf("wrap(%v) = %v, want it not to be fatal", in, got)
	}
}

func TestWrapPassesNilThrough(t *testing.T) {
	t.Parallel()

	if err := wrap(nil); err != nil {
		t.Errorf("wrap(nil) = %v, want nil", err)
	}
}

// TestAnOverrideKeepsFatalErrorsFatal drives a real override end to end: a
// backend answering the way cli.Client's classify does, through the
// gateway, to the exact check the Work board's error screen makes. A missed
// wrap on any override would pass this same shape of error through
// unclassified, and domain.IsFatal would stop seeing it -- which is what
// leaves the user looking at an empty board instead of a screen telling them
// to sign in.
func TestAnOverrideKeepsFatalErrorsFatal(t *testing.T) {
	t.Parallel()

	g := New(fakeBackend{getPR: func(context.Context, string, int) (gql.PullRequest, error) {
		return gql.PullRequest{}, github.Classify(github.ErrUnauthenticated, "gh: Bad credentials")
	}})

	_, err := g.GetPR(context.Background(), "kukv/octoscope", 1)
	if !domain.IsFatal(err) {
		t.Fatalf("GetPR() error = %v, want domain.IsFatal", err)
	}
}
