package api

import (
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/github"
)

func TestTokenPrefersGHTokenOverGitHubToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "from-gh")
	t.Setenv("GITHUB_TOKEN", "from-github")

	got, err := Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "from-gh" {
		t.Errorf("token = %q, want from-gh", got)
	}
}

func TestTokenFallsBackToGitHubToken(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "from-github")

	got, err := Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "from-github" {
		t.Errorf("token = %q, want from-github", got)
	}
}

// Without a token the user has to act, and the gateway's wrap turns this
// sentinel into the one the UI checks with domain.IsFatal. A plain error
// would be reported above the key bar and retried forever.
func TestTokenWithoutOneIsUnauthenticated(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	_, err := Token()
	if !errors.Is(err, github.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
}

// Whitespace around a token pasted into a shell profile would be sent in the
// Authorization header and rejected as a bad credential.
func TestTokenIsTrimmed(t *testing.T) {
	t.Setenv("GH_TOKEN", "  padded\n")

	got, err := Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "padded" {
		t.Errorf("token = %q, want padded", got)
	}
}
