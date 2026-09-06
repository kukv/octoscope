package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed review.graphql
var reviewContextQuery string

// repoArgs names the repository for a GraphQL call.
//
// GraphQL's repository() takes owner and name separately, unlike `gh pr`
// which takes the whole "owner/name" after --repo. When no repository was
// named -- the ordinary case of running octoscope inside a checkout -- there
// is nothing to split, and `gh api` fills the placeholders {owner} and {repo}
// from the working directory's remote.
//
// Those placeholders are only substituted in -F values, which is why this is
// the one place a value that is not a number goes through -F. Everything the
// user typed still goes through -f (see AddReviewThread).
func repoArgs(repo string) []string {
	if repo == "" {
		return []string{"-F", "owner={owner}", "-F", "name={repo}"}
	}
	owner, name, _ := strings.Cut(repo, "/")
	return []string{"-f", "owner=" + owner, "-f", "name=" + name}
}

type reviewContextResponse struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				HeadRefName string `json:"headRefName"`
				BaseRefName string `json:"baseRefName"`
				Additions   int    `json:"additions"`
				Deletions   int    `json:"deletions"`
				Reviews     struct {
					Nodes []struct {
						ID       string `json:"id"`
						Comments struct {
							Nodes []pendingCommentNode `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviews"`
				ReviewThreads struct {
					Nodes []threadNode `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

type threadNode struct {
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	// Line is null once the code a thread was written against has moved, so
	// it has to be a pointer to tell "no line" from "line 0".
	Line         *int   `json:"line"`
	OriginalLine int    `json:"originalLine"`
	DiffSide     string `json:"diffSide"`
	Comments     struct {
		Nodes []threadCommentNode `json:"nodes"`
	} `json:"comments"`
}

type threadCommentNode struct {
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	Author    struct {
		Login string `json:"login"`
	} `json:"author"`
	PullRequestReview struct {
		State string `json:"state"`
	} `json:"pullRequestReview"`
}

// PRReviewContext fetches, in one request, everything the diff view needs to
// draw and change a review.
func (c *Client) PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error) {
	args := append([]string{"api", "graphql", "-f", "query=" + reviewContextQuery},
		repoArgs(c.effectiveRepo(repo))...)
	args = append(args, "-F", "number="+strconv.Itoa(number))
	out, err := c.run(ctx, c.dir, args...)
	if err != nil {
		return gh.ReviewContext{}, err
	}
	var resp reviewContextResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.ReviewContext{}, fmt.Errorf("parse review context: %w", err)
	}
	pr := resp.Data.Repository.PullRequest
	rc := gh.ReviewContext{
		PullRequestID: pr.ID,
		Title:         pr.Title,
		Head:          pr.HeadRefName,
		Base:          pr.BaseRefName,
		Additions:     pr.Additions,
		Deletions:     pr.Deletions,
	}
	placed := map[string]bool{}
	for _, n := range pr.ReviewThreads.Nodes {
		t := n.toDomain()
		if t.Pending() {
			placed[threadPosition(t)] = true
		}
		rc.Threads = append(rc.Threads, t)
	}
	if len(pr.Reviews.Nodes) > 0 {
		rc.PendingID = pr.Reviews.Nodes[0].ID
		// The unsubmitted review's own comments are asked for as well as
		// read out of reviewThreads. They should be the same set -- a pending
		// comment does come back in both -- but the whole point of keeping the
		// review on GitHub is that a comment written here is still there
		// later, and a design that rests on one field's behaviour deserves
		// the second source. Anything reviewThreads did not carry is added.
		for _, c := range pr.Reviews.Nodes[0].Comments.Nodes {
			if t, ok := c.toThread(); ok && !placed[threadPosition(t)] {
				rc.Threads = append(rc.Threads, t)
			}
		}
	}
	return rc, nil
}

// threadPosition names where a thread sits, for deduplicating the same
// comment arriving from two fields of the same query.
func threadPosition(t gh.ReviewThread) string {
	return fmt.Sprintf("%s:%d:%d", t.Path, t.Line, t.Side)
}

// pendingCommentNode is one comment of the unsubmitted review, read straight
// off the review rather than out of reviewThreads.
type pendingCommentNode struct {
	Path     string `json:"path"`
	Line     *int   `json:"line"`
	DiffSide string `json:"diffSide"`
	Body     string `json:"body"`
}

// toThread turns it into a one-comment thread. A comment with no line has
// nowhere to be drawn, so it is skipped rather than landing on line 0.
func (c pendingCommentNode) toThread() (gh.ReviewThread, bool) {
	if c.Line == nil {
		return gh.ReviewThread{}, false
	}
	t := gh.ReviewThread{Path: c.Path, Line: *c.Line}
	if c.DiffSide == "LEFT" {
		t.Side = gh.SideLeft
	}
	t.Comments = []gh.ThreadComment{{Body: c.Body, Pending: true}}
	return t, true
}

func (n threadNode) toDomain() gh.ReviewThread {
	t := gh.ReviewThread{
		Path:     n.Path,
		Line:     n.OriginalLine,
		Resolved: n.IsResolved,
		Outdated: n.IsOutdated,
	}
	if n.Line != nil {
		t.Line = *n.Line
	}
	if n.DiffSide == "LEFT" {
		t.Side = gh.SideLeft
	}
	for _, c := range n.Comments.Nodes {
		t.Comments = append(t.Comments, gh.ThreadComment{
			Author:    gh.Author{Login: c.Author.Login},
			Body:      c.Body,
			CreatedAt: c.CreatedAt,
			// PENDING is the only review state that means "written but not
			// sent"; every other one means the comment is already public.
			Pending: c.PullRequestReview.State == "PENDING",
		})
	}
	return t
}
