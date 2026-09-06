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

//go:embed start_review.graphql
var startReviewMutation string

//go:embed add_thread.graphql
var addThreadMutation string

//go:embed submit_review.graphql
var submitReviewMutation string

//go:embed review_at_once.graphql
var reviewAtOnceMutation string

//go:embed discard_review.graphql
var discardReviewMutation string

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
func repoArgs(repo string) ([]string, error) {
	if repo == "" {
		return []string{"-F", "owner={owner}", "-F", "name={repo}"}, nil
	}
	owner, name, ok := strings.Cut(repo, "/")
	if !ok {
		return nil, fmt.Errorf("repo %q has no owner/name separator", repo)
	}
	return []string{"-f", "owner=" + owner, "-f", "name=" + name}, nil
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
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"reviews"`
				ReviewThreads struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
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

// PRReviewContext fetches everything the diff view needs to draw and change
// a review. It takes one request per page of review threads: GitHub caps a
// connection at 100, and `gh api --paginate` cannot follow a GraphQL cursor
// that sits below the top level, so the walk is here.
func (c *Client) PRReviewContext(ctx context.Context, repo string, number int) (gh.ReviewContext, error) {
	repoFields, err := repoArgs(c.effectiveRepo(repo))
	if err != nil {
		return gh.ReviewContext{}, err
	}

	var rc gh.ReviewContext
	cursor := ""
	for {
		args := append([]string{"api", "graphql", "-f", "query=" + reviewContextQuery}, repoFields...)
		args = append(args, "-F", "number="+strconv.Itoa(number))
		if cursor != "" {
			args = append(args, "-f", "after="+cursor)
		}
		out, err := c.run(ctx, c.dir, args...)
		if err != nil {
			return gh.ReviewContext{}, err
		}
		var resp reviewContextResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return gh.ReviewContext{}, fmt.Errorf("parse review context: %w", err)
		}
		pr := resp.Data.Repository.PullRequest

		// The pull request's own fields repeat on every page; taking them
		// from the first is enough, and taking them again is harmless.
		rc.PullRequestID = pr.ID
		rc.Title = pr.Title
		rc.Head = pr.HeadRefName
		rc.Base = pr.BaseRefName
		rc.Additions = pr.Additions
		rc.Deletions = pr.Deletions
		if len(pr.Reviews.Nodes) > 0 {
			rc.PendingID = pr.Reviews.Nodes[0].ID
		}
		for _, n := range pr.ReviewThreads.Nodes {
			rc.Threads = append(rc.Threads, n.toDomain())
		}

		if !pr.ReviewThreads.PageInfo.HasNextPage || pr.ReviewThreads.PageInfo.EndCursor == "" {
			return rc, nil
		}
		cursor = pr.ReviewThreads.PageInfo.EndCursor
	}
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

// The five mutations take no context. They are changes, not fetches: a
// comment that has been sent has been sent, so there is nothing to abandon
// half-way. The existing AddPRComment and ClosePR take none for the same
// reason (.claude/rules/go-style.md).

// StartReview opens an unsubmitted review on the pull request and returns its
// node id. A pending review is visible only to its author, so this is the id
// the rest of the session adds comments to.
func (c *Client) StartReview(pullRequestID string) (string, error) {
	out, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+startReviewMutation,
		"-f", "pullRequestId="+pullRequestID,
	)
	if err != nil {
		return "", err
	}
	var resp struct {
		Data struct {
			AddPullRequestReview struct {
				PullRequestReview struct {
					ID string `json:"id"`
				} `json:"pullRequestReview"`
			} `json:"addPullRequestReview"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return "", fmt.Errorf("parse start review: %w", err)
	}
	return resp.Data.AddPullRequestReview.PullRequestReview.ID, nil
}

// apiSide spells a side the way the GraphQL DiffSide enum does. It is the one
// place that knows those words (.claude/rules/architecture.md).
func apiSide(s gh.DiffSide) string {
	if s == gh.SideLeft {
		return "LEFT"
	}
	return "RIGHT"
}

// apiEvent spells an event the way PullRequestReviewEvent does.
func apiEvent(e gh.ReviewEvent) string {
	switch e {
	case gh.EventApprove:
		return "APPROVE"
	case gh.EventRequestChanges:
		return "REQUEST_CHANGES"
	default:
		return "COMMENT"
	}
}

// AddReviewThread attaches one line comment to an unsubmitted review.
func (c *Client) AddReviewThread(reviewID string, comment gh.PendingComment) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+addThreadMutation,
		"-f", "reviewId="+reviewID,
		"-f", "path="+comment.Path,
		"-F", "line="+strconv.Itoa(comment.Line),
		"-f", "side="+apiSide(comment.Side),
		"-f", "body="+comment.Body,
	)
	return err
}

// SubmitReview sends the unsubmitted review, with every comment on it.
func (c *Client) SubmitReview(reviewID string, event gh.ReviewEvent, body string) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+submitReviewMutation,
		"-f", "reviewId="+reviewID,
		"-f", "event="+apiEvent(event),
		"-f", "body="+body,
	)
	return err
}

// SubmitNewReview submits a review that has no unsubmitted comments waiting.
// addPullRequestReview takes an event, so creating and submitting is one
// call. Approving a diff you had nothing to say about is the commonest review
// there is, and it should not have to leave a pending review behind first.
func (c *Client) SubmitNewReview(pullRequestID string, event gh.ReviewEvent, body string) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+reviewAtOnceMutation,
		"-f", "pullRequestId="+pullRequestID,
		"-f", "event="+apiEvent(event),
		"-f", "body="+body,
	)
	return err
}

// DiscardReview throws the unsubmitted review away, comments and all.
func (c *Client) DiscardReview(reviewID string) error {
	_, err := c.run(context.Background(), c.dir, "api", "graphql",
		"-f", "query="+discardReviewMutation,
		"-f", "reviewId="+reviewID,
	)
	return err
}
