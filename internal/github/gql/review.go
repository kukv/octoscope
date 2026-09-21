package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"time"
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

//go:embed thread_comments.graphql
var threadCommentsQuery string

// ReviewContext is a pull request's review context as GraphQL answers it.
type ReviewContext struct {
	ID          string
	Title       string
	HeadRefName string
	BaseRefName string
	Additions   int
	Deletions   int
	// PendingID is the unsubmitted review's node id, empty when there is
	// none. A pending review is visible only to its author, so anything that
	// comes back here belongs to the viewer.
	PendingID string
	Threads   []ReviewThread
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
					Nodes []ReviewThread `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

// ReviewThread is one conversation attached to a line of the diff, as
// GraphQL answers it.
type ReviewThread struct {
	// ID is not carried past this package: only thread_comments.graphql
	// uses it, to page a thread's own comments.
	ID         string `json:"id"`
	IsResolved bool   `json:"isResolved"`
	IsOutdated bool   `json:"isOutdated"`
	Path       string `json:"path"`
	// Line is null once the code a thread was written against has moved, so
	// it has to be a pointer to tell "no line" from "line 0".
	Line         *int   `json:"line"`
	OriginalLine int    `json:"originalLine"`
	DiffSide     string `json:"diffSide"`
	Comments     struct {
		PageInfo PageInfo        `json:"pageInfo"`
		Nodes    []ThreadComment `json:"nodes"`
	} `json:"comments"`
}

// ThreadComment is one comment inside a review thread, as GraphQL answers it.
type ThreadComment struct {
	Body              string    `json:"body"`
	CreatedAt         time.Time `json:"createdAt"`
	Author            Author    `json:"author"`
	PullRequestReview struct {
		State string `json:"state"`
	} `json:"pullRequestReview"`
}

// PRReviewContext fetches everything the diff view needs to draw and change
// a review. It walks review threads one page at a time itself, since
// `gh api --paginate` cannot follow a GraphQL cursor below the top level,
// and follows a thread's own comments only when that thread says it has
// more.
func (c *Client) PRReviewContext(ctx context.Context, repo string, number int) (ReviewContext, error) {
	repoFields, err := c.repoVars(repo)
	if err != nil {
		return ReviewContext{}, err
	}

	var rc ReviewContext
	var nodes []ReviewThread
	cursor := ""
	for {
		vars := append(slices.Clone(repoFields), N("number", number))
		if cursor != "" {
			vars = append(vars, S("after", cursor))
		}
		out, err := c.Read(ctx, reviewContextQuery, vars...)
		if err != nil {
			return ReviewContext{}, err
		}
		var resp reviewContextResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return ReviewContext{}, fmt.Errorf("parse review context: %w", err)
		}
		pr := resp.Data.Repository.PullRequest

		// The pull request's own fields repeat on every page; taking them
		// from the first is enough, and taking them again is harmless.
		rc.ID = pr.ID
		rc.Title = pr.Title
		rc.HeadRefName = pr.HeadRefName
		rc.BaseRefName = pr.BaseRefName
		rc.Additions = pr.Additions
		rc.Deletions = pr.Deletions
		if len(pr.Reviews.Nodes) > 0 {
			rc.PendingID = pr.Reviews.Nodes[0].ID
		}
		nodes = append(nodes, pr.ReviewThreads.Nodes...)

		if !pr.ReviewThreads.PageInfo.HasNextPage || pr.ReviewThreads.PageInfo.EndCursor == "" {
			break
		}
		cursor = pr.ReviewThreads.PageInfo.EndCursor
	}

	for _, n := range nodes {
		if n.Comments.PageInfo.HasNextPage && n.Comments.PageInfo.EndCursor != "" {
			rest, err := c.threadComments(ctx, n.ID, n.Comments.PageInfo.EndCursor)
			if err != nil {
				return ReviewContext{}, fmt.Errorf("fetch thread comments: %w", err)
			}
			n.Comments.Nodes = append(n.Comments.Nodes, rest...)
		}
		rc.Threads = append(rc.Threads, n)
	}
	return rc, nil
}

type threadCommentsResponse struct {
	Data struct {
		Node struct {
			Comments struct {
				PageInfo PageInfo        `json:"pageInfo"`
				Nodes    []ThreadComment `json:"nodes"`
			} `json:"comments"`
		} `json:"node"`
	} `json:"data"`
}

// threadComments reads what did not fit in the page PRReviewContext already
// has, starting after the cursor that page ended on.
func (c *Client) threadComments(ctx context.Context, threadID, after string) ([]ThreadComment, error) {
	var rest []ThreadComment
	cursor := after
	for {
		out, err := c.Read(ctx, threadCommentsQuery, S("threadId", threadID), S("after", cursor))
		if err != nil {
			return nil, err
		}
		var resp threadCommentsResponse
		if err := json.Unmarshal(out, &resp); err != nil {
			return nil, fmt.Errorf("parse thread comments: %w", err)
		}
		page := resp.Data.Node.Comments
		rest = append(rest, page.Nodes...)
		if !page.PageInfo.HasNextPage || page.PageInfo.EndCursor == "" {
			return rest, nil
		}
		cursor = page.PageInfo.EndCursor
	}
}

// StartReview opens an unsubmitted review on the pull request and returns its
// node id. A pending review is visible only to its author, so this is the id
// the rest of the session adds comments to.
func (c *Client) StartReview(ctx context.Context, pullRequestID string) (string, error) {
	out, err := c.Write(ctx, startReviewMutation, S("pullRequestId", pullRequestID))
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

// PendingComment is a line comment on its way to GitHub.
type PendingComment struct {
	Path string
	Line int
	// Side spells which version of the file the comment sits on, the way
	// GraphQL's DiffSide enum does.
	Side string
	Body string
}

// ReviewEvent is what submitting a review says about it, spelled the way
// GraphQL's PullRequestReviewEvent enum does.
type ReviewEvent string

const (
	EventApprove        ReviewEvent = "APPROVE"
	EventRequestChanges ReviewEvent = "REQUEST_CHANGES"
	EventComment        ReviewEvent = "COMMENT"
)

// AddReviewThread attaches one line comment to an unsubmitted review.
func (c *Client) AddReviewThread(ctx context.Context, reviewID string, comment PendingComment) error {
	_, err := c.Write(ctx, addThreadMutation,
		S("reviewId", reviewID),
		S("path", comment.Path),
		N("line", comment.Line),
		S("side", comment.Side),
		S("body", comment.Body),
	)
	return err
}

// SubmitReview sends the unsubmitted review, with every comment on it.
func (c *Client) SubmitReview(ctx context.Context, reviewID string, event ReviewEvent, body string) error {
	_, err := c.Write(ctx, submitReviewMutation,
		S("reviewId", reviewID),
		S("event", string(event)),
		S("body", body),
	)
	return err
}

// SubmitNewReview submits a review that has no unsubmitted comments waiting.
// addPullRequestReview takes an event, so creating and submitting is one
// call. Approving a diff you had nothing to say about is the commonest review
// there is, and it should not have to leave a pending review behind first.
func (c *Client) SubmitNewReview(ctx context.Context, pullRequestID string, event ReviewEvent, body string) error {
	_, err := c.Write(ctx, reviewAtOnceMutation,
		S("pullRequestId", pullRequestID),
		S("event", string(event)),
		S("body", body),
	)
	return err
}

// DiscardReview throws the unsubmitted review away, comments and all.
func (c *Client) DiscardReview(ctx context.Context, reviewID string) error {
	_, err := c.Write(ctx, discardReviewMutation, S("reviewId", reviewID))
	return err
}
