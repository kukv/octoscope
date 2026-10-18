// Package cli fetches GitHub data by running the gh CLI in a target directory.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// listLimit is how many items gh label list is asked for. It fetches 30 by
// default and says nothing about the rest, so a repository with more labels
// than that would lose them without a word. gh names no upper bound; it pages
// until it has as many as it was asked for.
const listLimit = "100"

// callTimeout bounds one gh call. Most screens hand this package
// context.Background(), so without it a gh that hangs leaves a loading screen
// up for good. The value is the api backend's responseTimeout, for the same
// reason: the longest answer measured there was 8.13s, rounded up to the next
// power of two.
var callTimeout = 16 * time.Second

// logTimeout bounds gh run view --log, which downloads the whole run's log
// archive to show one job. Uncached, microsoft/vscode's Code OSS runs (38
// jobs each) took 10.8s to 17.1s for a 9 MB archive and 21.2s at worst for an
// 18 MB one, so callTimeout would cut ordinary logs off. This is about five
// times the worst of those.
var logTimeout = 2 * time.Minute

// runFunc must return stdout even when err is non-nil: gh api graphql exits
// non-zero when the body carries a top-level "errors" array, and that body
// still holds the data GitHub could answer for. Only callers that can salvage
// a partial body (RepoCounts) read it; everyone else looks at err first.
type runFunc func(ctx context.Context, dir string, args ...string) ([]byte, error)

// Client runs gh commands in a fixed directory, against a fixed repository.
type Client struct {
	*gql.Client
	dir  string
	repo string
	run  runFunc
}

// New returns a client for the repository named by repo ("owner/name").
// An empty repo falls back to the repository of the git remote in dir.
func New(dir, repo string) *Client {
	c := &Client{dir: dir, repo: repo, run: runGh}
	c.Client = &gql.Client{
		// The closure reads c.run at call time: tests replace it after New.
		Do: func(ctx context.Context, doc string, vars []gql.Var) ([]byte, error) {
			return c.exec(ctx, callTimeout, ghArgs(doc, vars)...)
		},
		RepoVars: func(repo string) ([]gql.Var, error) { return repoVars(c.effectiveRepo(repo)) },
	}
	return c
}

// repoVars names the repository for a GraphQL call. GraphQL's repository()
// takes owner and name separately, unlike `gh pr` which takes the whole
// "owner/name" after --repo. When no repository was named -- the ordinary
// case of running octoscope inside a checkout -- there is nothing to split,
// and gh fills the placeholders from the working directory's remote.
//
// A named repository is split by gql.SplitRepoVars, the same parse the api
// backend uses: the string reaches both of them straight off --repo or a
// hand-edited settings file, and one backend accepting what the other
// refuses would send GitHub a repository nobody meant.
func repoVars(repo string) ([]gql.Var, error) {
	if repo == "" {
		return []gql.Var{gql.Placeholder("owner", "{owner}"), gql.Placeholder("name", "{repo}")}, nil
	}
	return gql.SplitRepoVars(repo)
}

// effectiveRepo picks the per-call repository if given, else the client's.
func (c *Client) effectiveRepo(repo string) string {
	if repo != "" {
		return repo
	}
	return c.repo
}

// classify names the failures a caller acts on differently: one worth
// asking again for, and one only the user can fix. Everything else keeps
// the text gh printed and no type at all.
func classify(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, status := range []string{"HTTP 502", "HTTP 503", "HTTP 504"} {
		if strings.Contains(msg, status) {
			return github.Classify(github.ErrTransient, msg)
		}
	}
	if strings.Contains(msg, "Bad credentials") || strings.Contains(msg, "gh auth login") {
		return github.Classify(github.ErrUnauthenticated, msg)
	}
	return err
}

func runGh(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, github.ErrNotInstalled
	}
	// gh subcommand args are built internally from typed values (subcommand,
	// numbers, flags), never from untrusted external input.
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// A child gh leaves behind can hold stdout open after gh itself is
	// killed, and Run would wait on that copy for as long as the child lives.
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return stdout.Bytes(), classify(fmt.Errorf("gh %s: %s", args[0], msg))
		}
		return stdout.Bytes(), classify(fmt.Errorf("gh %s: %w", args[0], err))
	}
	return stdout.Bytes(), nil
}

// errNoAnswer is the cause exec gives its own deadline, so that one can be
// told apart from a deadline or cancellation the caller set.
var errNoAnswer = errors.New("gh did not answer in time")

// timedOut is a gh call this package stopped. It reads as transient to the
// UI, which puts it on the one-line notice like a 502, and as a deadline to
// the read paths, which must not ask again: a gh that hung once is not one a
// second try will rescue, and a retry doubles the wait.
type timedOut struct{ msg string }

func (e *timedOut) Error() string   { return e.msg }
func (e *timedOut) Unwrap() []error { return []error{github.ErrTransient, context.DeadlineExceeded} }

// exec runs one gh call for at most limit. The limit is decided by the
// context's cause rather than by the error: a killed gh reports
// "signal: killed", not context.DeadlineExceeded.
func (c *Client) exec(ctx context.Context, limit time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, limit, errNoAnswer)
	defer cancel()
	out, err := c.run(ctx, c.dir, args...)
	if err != nil && errors.Is(context.Cause(ctx), errNoAnswer) {
		return out, &timedOut{msg: fmt.Sprintf("gh %s: no answer in %s", args[0], limit)}
	}
	return out, err
}

// read runs a gh call that only reads, asking again once when GitHub's front
// end did not answer. Only reads take this path: a 502 says no answer came
// back, not that nothing arrived, so a repeated write could apply twice.
func (c *Client) read(ctx context.Context, limit time.Duration, args ...string) ([]byte, error) {
	out, err := c.exec(ctx, limit, args...)
	if err == nil || ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, github.ErrTransient) {
		return out, err
	}
	return c.exec(ctx, limit, args...)
}

func appendRepo(args []string, repo string) []string {
	if repo != "" {
		return append(args, "--repo", repo)
	}
	return args
}

// RepoName returns the repository the client works against, as GitHub spells
// it. An empty repo means the working directory's, which gh fills in.
func (c *Client) RepoName(ctx context.Context) (string, error) {
	return c.Client.RepoName(ctx, c.repo)
}

func (c *Client) AddPRComment(ctx context.Context, repo string, number int, body string) error {
	_, err := c.exec(ctx, callTimeout, appendRepo([]string{"pr", "comment", strconv.Itoa(number), "--body", body}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) AddIssueComment(ctx context.Context, repo string, number int, body string) error {
	_, err := c.exec(ctx, callTimeout, appendRepo([]string{"issue", "comment", strconv.Itoa(number), "--body", body}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ClosePR(ctx context.Context, repo string, number int) error {
	_, err := c.exec(ctx, callTimeout, appendRepo([]string{"pr", "close", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ReopenPR(ctx context.Context, repo string, number int) error {
	_, err := c.exec(ctx, callTimeout, appendRepo([]string{"pr", "reopen", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) CloseIssue(ctx context.Context, repo string, number int) error {
	_, err := c.exec(ctx, callTimeout, appendRepo([]string{"issue", "close", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ReopenIssue(ctx context.Context, repo string, number int) error {
	_, err := c.exec(ctx, callTimeout, appendRepo([]string{"issue", "reopen", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ListLabels(ctx context.Context, repo string) ([]gql.Label, error) {
	args := appendRepo([]string{"label", "list", "--json", "name,color", "--limit", listLimit}, c.effectiveRepo(repo))
	out, err := c.read(ctx, callTimeout, args...)
	if err != nil {
		return nil, err
	}
	var labels []gql.Label
	if err := json.Unmarshal(out, &labels); err != nil {
		return nil, fmt.Errorf("parse label list: %w", err)
	}
	return labels, nil
}

// ListAssignees returns the logins of users assignable on the repository.
// gh api substitutes {owner}/{repo} from the current directory's repo; for an
// override we build the explicit path (gh api takes no --repo).
//
// per_page=100 is REST's maximum, and the request is not paged: a repository
// with more than 100 assignable users would give the picker a list nobody
// could pick from anyway.
func (c *Client) ListAssignees(ctx context.Context, repo string) ([]string, error) {
	path := "repos/{owner}/{repo}/assignees?per_page=100"
	if r := c.effectiveRepo(repo); r != "" {
		path = "repos/" + r + "/assignees?per_page=100"
	}
	out, err := c.read(ctx, callTimeout, "api", path)
	if err != nil {
		return nil, err
	}
	var users []gql.Author
	if err := json.Unmarshal(out, &users); err != nil {
		return nil, fmt.Errorf("parse assignees: %w", err)
	}
	logins := make([]string, len(users))
	for i, u := range users {
		logins[i] = u.Login
	}
	return logins, nil
}

func (c *Client) editItems(ctx context.Context, kindCmd, repo string, number int, add, remove []string, addFlag, removeFlag string) error {
	args := []string{kindCmd, "edit", strconv.Itoa(number)}
	for _, v := range add {
		args = append(args, addFlag, v)
	}
	for _, v := range remove {
		args = append(args, removeFlag, v)
	}
	_, err := c.exec(ctx, callTimeout, appendRepo(args, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) EditPRLabels(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editItems(ctx, "pr", repo, number, add, remove, "--add-label", "--remove-label")
}

func (c *Client) EditIssueLabels(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editItems(ctx, "issue", repo, number, add, remove, "--add-label", "--remove-label")
}

func (c *Client) EditPRAssignees(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editItems(ctx, "pr", repo, number, add, remove, "--add-assignee", "--remove-assignee")
}

func (c *Client) EditIssueAssignees(ctx context.Context, repo string, number int, add, remove []string) error {
	return c.editItems(ctx, "issue", repo, number, add, remove, "--add-assignee", "--remove-assignee")
}

// ghArgs spells one document and its variables the way gh api graphql takes
// them. -F is for values gh has to parse rather than pass through: numbers,
// and the {owner}/{repo} placeholders it fills from the working directory.
func ghArgs(doc string, vars []gql.Var) []string {
	args := []string{"api", "graphql", "-f", "query=" + doc}
	for _, v := range vars {
		switch v.Kind {
		case gql.VarInt:
			args = append(args, "-F", v.Name+"="+strconv.Itoa(v.Int))
		case gql.VarPlaceholder:
			args = append(args, "-F", v.Name+"="+v.Str)
		default:
			args = append(args, "-f", v.Name+"="+v.Str)
		}
	}
	return args
}
