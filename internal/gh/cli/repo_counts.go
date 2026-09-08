package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed repo_counts.graphql
var repoCountSelection string

// buildRepoCountsQuery writes one aliased repository selection per name. The
// names themselves never enter the document: they travel as the variables
// this declares, so a name from the settings file cannot become part of the
// query.
func buildRepoCountsQuery(n int) string {
	var decls, body strings.Builder
	for i := range n {
		if i > 0 {
			decls.WriteString(", ")
		}
		fmt.Fprintf(&decls, "$o%d: String!, $n%d: String!", i, i)
		selection := strings.NewReplacer(
			"$OWNER", fmt.Sprintf("$o%d", i),
			"$NAME", fmt.Sprintf("$n%d", i),
		).Replace(repoCountSelection)
		fmt.Fprintf(&body, "  r%d: %s\n", i, strings.TrimSpace(selection))
	}
	return "query (" + decls.String() + ") {\n" + body.String() + "}\n"
}

type repoCountsResponse struct {
	Data map[string]*struct {
		NameWithOwner string `json:"nameWithOwner"`
		PullRequests  struct {
			TotalCount int `json:"totalCount"`
		} `json:"pullRequests"`
		Issues struct {
			TotalCount int `json:"totalCount"`
		} `json:"issues"`
	} `json:"data"`
}

// RepoCounts fetches how many pull requests and issues are open in each
// repository, in one request. A repository whose name does not split into
// owner/name, or that GitHub could not resolve, comes back Unavailable
// instead of failing the rest.
func (c *Client) RepoCounts(ctx context.Context, repos []string) ([]gh.RepoCount, error) {
	counts := make([]gh.RepoCount, len(repos))
	args := []string{}
	indices := []int{}
	for i, repo := range repos {
		counts[i].Repo = repo
		owner, name, ok := strings.Cut(repo, "/")
		if !ok {
			counts[i].Unavailable = true
			continue
		}
		n := len(indices)
		args = append(args, "-f", fmt.Sprintf("o%d=%s", n, owner), "-f", fmt.Sprintf("n%d=%s", n, name))
		indices = append(indices, i)
	}
	if len(indices) == 0 {
		return counts, nil
	}

	query := buildRepoCountsQuery(len(indices))
	callArgs := append([]string{"api", "graphql", "-f", "query=" + query}, args...)
	// gh api graphql exits non-zero when any alias could not be resolved, but
	// the body still carries the data GitHub could answer for, so a run
	// error is not fatal here; only a body that fails to parse is.
	out, runErr := c.run(ctx, c.dir, callArgs...)
	var resp repoCountsResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		if runErr != nil {
			return nil, runErr
		}
		return nil, fmt.Errorf("parse repo counts: %w", err)
	}
	// A response with no "data" at all means the document itself was
	// rejected -- bad credentials, rate limiting, a validation error -- not
	// that some repositories answered and others did not. Reading it as "no
	// repository resolved" would tell a user whose token expired that every
	// repository disappeared.
	if resp.Data == nil && runErr != nil {
		return nil, runErr
	}
	for n, i := range indices {
		alias := resp.Data[fmt.Sprintf("r%d", n)]
		if alias == nil {
			counts[i].Unavailable = true
			continue
		}
		counts[i].PRs = alias.PullRequests.TotalCount
		counts[i].Issues = alias.Issues.TotalCount
	}
	return counts, nil
}
