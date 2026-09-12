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

	"github.com/kukv/octoscope/internal/browser"
	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/gh/gql"
)

const (
	prListFields = "number,title,author,state,isDraft,updatedAt,reviewDecision,url," +
		"labels,headRefName,baseRefName,additions,deletions,statusCheckRollup"
	prViewFields    = prListFields + ",body,comments,labels,assignees"
	issueListFields = "number,title,author,state,updatedAt,labels,url"
	issueViewFields = issueListFields + ",body,comments,assignees"
)

// listLimit is how many items the gh list subcommands are asked for. Every
// one of them -- pr list, issue list, label list -- fetches 30 by default and
// says nothing about the rest, so a repository with more open pull requests
// than that would lose them without a word. gh names no upper bound; it pages
// until it has as many as it was asked for.
const listLimit = "100"

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
			return c.run(ctx, c.dir, ghArgs(doc, vars)...)
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
func repoVars(repo string) ([]gql.Var, error) {
	if repo == "" {
		return []gql.Var{gql.Placeholder("owner", "{owner}"), gql.Placeholder("name", "{repo}")}, nil
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("repo %q has no owner/name separator", repo)
	}
	return []gql.Var{gql.S("owner", owner), gql.S("name", name)}, nil
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
			return gh.Classify(gh.ErrTransient, msg)
		}
	}
	if strings.Contains(msg, "Bad credentials") || strings.Contains(msg, "gh auth login") {
		return gh.Classify(gh.ErrUnauthenticated, msg)
	}
	return err
}

func runGh(ctx context.Context, dir string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, gh.ErrGhNotFound
	}
	// gh subcommand args are built internally from typed values (subcommand,
	// numbers, flags), never from untrusted external input.
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return stdout.Bytes(), classify(fmt.Errorf("gh %s: %s", args[0], msg))
		}
		return stdout.Bytes(), classify(fmt.Errorf("gh %s: %w", args[0], err))
	}
	return stdout.Bytes(), nil
}

// read runs a gh call that only reads, asking again once when GitHub's front
// end did not answer. Only reads take this path: a 502 says no answer came
// back, not that nothing arrived, so a repeated write could apply twice.
func (c *Client) read(ctx context.Context, dir string, args ...string) ([]byte, error) {
	out, err := c.run(ctx, dir, args...)
	if err == nil || ctx.Err() != nil || !errors.Is(err, gh.ErrTransient) {
		return out, err
	}
	return c.run(ctx, dir, args...)
}

func appendRepo(args []string, repo string) []string {
	if repo != "" {
		return append(args, "--repo", repo)
	}
	return args
}

func (c *Client) ListPRs(ctx context.Context, repo string) ([]gh.PR, error) {
	args := appendRepo([]string{"pr", "list", "--json", prListFields, "--limit", listLimit}, c.effectiveRepo(repo))
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	var prs []prJSON
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse pr list: %w", err)
	}
	return toPRs(prs), nil
}

func (c *Client) ListIssues(ctx context.Context, repo string) ([]gh.Issue, error) {
	args := appendRepo([]string{"issue", "list", "--json", issueListFields, "--limit", listLimit}, c.effectiveRepo(repo))
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	var issues []issueJSON
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parse issue list: %w", err)
	}
	return toIssues(issues), nil
}

func (c *Client) GetPR(ctx context.Context, repo string, number int) (gh.PR, error) {
	args := appendRepo([]string{"pr", "view", strconv.Itoa(number), "--json", prViewFields}, c.effectiveRepo(repo))
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return gh.PR{}, err
	}
	var pr prJSON
	if err := json.Unmarshal(out, &pr); err != nil {
		return gh.PR{}, fmt.Errorf("parse pr view: %w", err)
	}
	return pr.toDomain(), nil
}

func (c *Client) GetIssue(ctx context.Context, repo string, number int) (gh.Issue, error) {
	args := appendRepo([]string{"issue", "view", strconv.Itoa(number), "--json", issueViewFields}, c.effectiveRepo(repo))
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return gh.Issue{}, err
	}
	var issue issueJSON
	if err := json.Unmarshal(out, &issue); err != nil {
		return gh.Issue{}, fmt.Errorf("parse issue view: %w", err)
	}
	return issue.toDomain(), nil
}

func (c *Client) RepoName(ctx context.Context) (string, error) {
	args := []string{"repo", "view"}
	if c.repo != "" {
		args = append(args, c.repo)
	}
	args = append(args, "--json", "nameWithOwner")
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return "", err
	}
	var v struct {
		NameWithOwner string `json:"nameWithOwner"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return "", fmt.Errorf("parse repo view: %w", err)
	}
	return v.NameWithOwner, nil
}

// OpenWeb shows the item in a browser. It does not go through gh: `gh ... --web`
// looks only for xdg-open, x-www-browser, www-browser and wslview, and a WSL
// machine has none of them -- wslu, which provides wslview, is no longer
// packaged for Ubuntu. GitHub gives every item its URL, so there is nothing
// gh would add here.
func (c *Client) OpenWeb(url string) error {
	return browser.Open(url)
}

func (c *Client) AddPRComment(repo string, number int, body string) error {
	_, err := c.run(context.Background(), c.dir, appendRepo([]string{"pr", "comment", strconv.Itoa(number), "--body", body}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) AddIssueComment(repo string, number int, body string) error {
	_, err := c.run(context.Background(), c.dir, appendRepo([]string{"issue", "comment", strconv.Itoa(number), "--body", body}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ClosePR(repo string, number int) error {
	_, err := c.run(context.Background(), c.dir, appendRepo([]string{"pr", "close", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ReopenPR(repo string, number int) error {
	_, err := c.run(context.Background(), c.dir, appendRepo([]string{"pr", "reopen", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) CloseIssue(repo string, number int) error {
	_, err := c.run(context.Background(), c.dir, appendRepo([]string{"issue", "close", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ReopenIssue(repo string, number int) error {
	_, err := c.run(context.Background(), c.dir, appendRepo([]string{"issue", "reopen", strconv.Itoa(number)}, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) ListLabels(ctx context.Context, repo string) ([]gh.Label, error) {
	args := appendRepo([]string{"label", "list", "--json", "name,color", "--limit", listLimit}, c.effectiveRepo(repo))
	out, err := c.read(ctx, c.dir, args...)
	if err != nil {
		return nil, err
	}
	var labels []gh.Label
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
	out, err := c.read(ctx, c.dir, "api", path)
	if err != nil {
		return nil, err
	}
	var users []gh.Author
	if err := json.Unmarshal(out, &users); err != nil {
		return nil, fmt.Errorf("parse assignees: %w", err)
	}
	logins := make([]string, len(users))
	for i, u := range users {
		logins[i] = u.Login
	}
	return logins, nil
}

func (c *Client) editItems(kindCmd, repo string, number int, add, remove []string, addFlag, removeFlag string) error {
	args := []string{kindCmd, "edit", strconv.Itoa(number)}
	for _, v := range add {
		args = append(args, addFlag, v)
	}
	for _, v := range remove {
		args = append(args, removeFlag, v)
	}
	_, err := c.run(context.Background(), c.dir, appendRepo(args, c.effectiveRepo(repo))...)
	return err
}

func (c *Client) EditPRLabels(repo string, number int, add, remove []string) error {
	return c.editItems("pr", repo, number, add, remove, "--add-label", "--remove-label")
}

func (c *Client) EditIssueLabels(repo string, number int, add, remove []string) error {
	return c.editItems("issue", repo, number, add, remove, "--add-label", "--remove-label")
}

func (c *Client) EditPRAssignees(repo string, number int, add, remove []string) error {
	return c.editItems("pr", repo, number, add, remove, "--add-assignee", "--remove-assignee")
}

func (c *Client) EditIssueAssignees(repo string, number int, add, remove []string) error {
	return c.editItems("issue", repo, number, add, remove, "--add-assignee", "--remove-assignee")
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
