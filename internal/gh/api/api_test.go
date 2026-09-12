package api

import (
	"errors"
	"testing"

	"github.com/kukv/octoscope/internal/gh"
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

// Without a token the user has to act, and the UI tells them so by asking
// gh.IsFatal. A plain error would be reported above the key bar and retried
// forever.
func TestTokenWithoutOneIsUnauthenticated(t *testing.T) {
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")

	_, err := Token()
	if !errors.Is(err, gh.ErrUnauthenticated) {
		t.Fatalf("err = %v, want ErrUnauthenticated", err)
	}
	if !gh.IsFatal(err) {
		t.Error("the UI would not show the error screen for this")
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
