package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

// remoteTimeout bounds the one local subprocess this package runs. gh's own
// lookup is allowed twenty seconds because it reaches the API; reading a
// remote out of a git config does not.
const remoteTimeout = 5 * time.Second

// repoVars names the repository of a call. An explicit repository -- the
// --repo flag, or the sidebar asking for one of its rows -- wins; otherwise
// the working directory's remote says where we are, the way gh resolves it.
func (c *Client) repoVars(repo string) ([]gql.Var, error) {
	if repo == "" {
		repo = c.repo
	}
	if repo == "" {
		var err error
		if repo, err = c.currentRepo(); err != nil {
			return nil, err
		}
	}
	return gql.SplitRepoVars(repo)
}

// currentRepo reads the working directory's origin remote, once. The sidebar
// asks for several repositories at a time and every refresh would otherwise
// pay for a subprocess per row.
//
// gql.Client.RepoVars takes no context, so the deadline is this package's own.
func (c *Client) currentRepo() (string, error) {
	c.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
		defer cancel()
		out, err := c.git()(ctx, c.dir, "remote", "get-url", "origin")
		if err != nil {
			c.currentErr = fmt.Errorf("read the origin remote: %w", err)
			return
		}
		c.current, c.currentErr = parseRemote(string(out))
	})
	return c.current, c.currentErr
}

// parseRemote reads "owner/name" out of a remote URL. git writes the same
// remote in several shapes -- scp-like ssh, an ssh:// URL, https with or
// without the .git suffix -- and all of them reach this.
//
// Anything but github.com is refused: GitHub Enterprise is out of scope, and
// reading a gitlab remote as a github repository would ask GitHub for one
// that does not exist.
func parseRemote(url string) (string, error) {
	s := strings.TrimSpace(url)
	const host = "github.com"

	switch {
	case strings.HasPrefix(s, "git@"+host+":"):
		s = strings.TrimPrefix(s, "git@"+host+":")
	default:
		// Strip the scheme and any user@ in front of the host.
		if _, rest, ok := strings.Cut(s, "://"); ok {
			s = rest
		}
		if _, rest, ok := strings.Cut(s, "@"); ok {
			s = rest
		}
		rest, ok := strings.CutPrefix(s, host+"/")
		if !ok {
			return "", fmt.Errorf("remote %q is not on %s", url, host)
		}
		s = rest
	}
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(s), "/"), ".git")
	if _, _, ok := gh.SplitRepo(s); !ok {
		return "", fmt.Errorf("remote %q has no owner/name", url)
	}
	return s, nil
}

// RepoName returns the repository the client works against, as GitHub spells
// it. A failure means the directory has no repository to speak of, which the
// Repos tab reads as "there is no current repository" rather than as a fault.
func (c *Client) RepoName(ctx context.Context) (string, error) {
	return c.Client.RepoName(ctx, c.repo)
}

// git is the subprocess runner. Tests replace it.
func (c *Client) git() gitFunc {
	if c.runGit != nil {
		return c.runGit
	}
	return runGit
}

type gitFunc func(ctx context.Context, dir string, args ...string) ([]byte, error)

func runGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	// The arguments are this package's own literals, never external input.
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return nil, errors.New(string(msg))
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}
