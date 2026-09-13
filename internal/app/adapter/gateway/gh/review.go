package gh

import (
	"context"

	"github.com/kukv/octoscope/internal/app/domain"
	"github.com/kukv/octoscope/internal/github"
	"github.com/kukv/octoscope/internal/github/gql"
)

// PRDiff returns the pull request's diff, one entry per file.
func (g *Gateway) PRDiff(ctx context.Context, repo string, number int) ([]domain.FileDiff, error) {
	d, err := g.backend.PRDiff(ctx, repo, number)
	if err != nil {
		return nil, err
	}
	if d.Files != nil {
		files := make([]domain.FileDiff, len(d.Files))
		for i, f := range d.Files {
			files[i] = toFileDiff(f)
		}
		return files, nil
	}
	return domain.ParseDiff(d.Raw), nil
}

// PRReviewContext returns everything the diff view needs to draw and change
// a review.
func (g *Gateway) PRReviewContext(ctx context.Context, repo string, number int) (domain.ReviewContext, error) {
	rc, err := g.backend.PRReviewContext(ctx, repo, number)
	if err != nil {
		return domain.ReviewContext{}, err
	}
	return toReviewContext(rc), nil
}

// AddReviewThread attaches one line comment to an unsubmitted review.
func (g *Gateway) AddReviewThread(reviewID string, c domain.PendingComment) error {
	return g.backend.AddReviewThread(reviewID, fromPendingComment(c))
}

// SubmitReview sends the unsubmitted review, with every comment on it.
func (g *Gateway) SubmitReview(reviewID string, event domain.ReviewEvent, body string) error {
	return g.backend.SubmitReview(reviewID, fromReviewEvent(event), body)
}

// SubmitNewReview submits a review that has no unsubmitted comments waiting.
func (g *Gateway) SubmitNewReview(pullRequestID string, event domain.ReviewEvent, body string) error {
	return g.backend.SubmitNewReview(pullRequestID, fromReviewEvent(event), body)
}

func fromPendingComment(c domain.PendingComment) gql.PendingComment {
	return gql.PendingComment{
		Path: c.Path,
		Line: c.Line,
		Side: fromDiffSide(c.Side),
		Body: c.Body,
	}
}

// fromDiffSide spells a side the way the GraphQL DiffSide enum does. It is
// the one place that knows those words (.claude/rules/architecture.md).
func fromDiffSide(s domain.DiffSide) string {
	if s == domain.SideLeft {
		return "LEFT"
	}
	return "RIGHT"
}

// fromReviewEvent spells an event the way PullRequestReviewEvent does.
func fromReviewEvent(e domain.ReviewEvent) gql.ReviewEvent {
	switch e {
	case domain.EventApprove:
		return gql.EventApprove
	case domain.EventRequestChanges:
		return gql.EventRequestChanges
	default:
		return gql.EventComment
	}
}

// toFileDiff converts one files-API entry. Patch is a pointer because GitHub
// omits the field entirely for a file it declines to send a diff for (too
// large, or binary); that is PatchOmitted, not Binary, since the files API
// gives no way to tell a binary file apart from any other reason GitHub left
// the patch out.
func toFileDiff(f github.PRFile) domain.FileDiff {
	fd := domain.FileDiff{
		Path:      f.Filename,
		OldPath:   f.PreviousFilename,
		Status:    toFileStatus(f.Status),
		Additions: f.Additions,
		Deletions: f.Deletions,
	}
	if f.Patch == nil {
		fd.PatchOmitted = true
		return fd
	}
	fd.Hunks = domain.ParseBarePatch(*f.Patch)
	return fd
}

// toFileStatus translates the files API's status spelling. GitHub says
// "removed", not "deleted"; anything unrecognised (including "modified")
// falls back to FileModified.
func toFileStatus(s string) domain.FileStatus {
	switch s {
	case "added":
		return domain.FileAdded
	case "removed":
		return domain.FileDeleted
	case "renamed":
		return domain.FileRenamed
	case "copied":
		return domain.FileCopied
	case "changed":
		return domain.FileChanged
	case "unchanged":
		return domain.FileUnchanged
	default:
		return domain.FileModified
	}
}

func toReviewContext(rc gql.ReviewContext) domain.ReviewContext {
	dc := domain.ReviewContext{
		PullRequestID: rc.ID,
		Title:         rc.Title,
		Head:          rc.HeadRefName,
		Base:          rc.BaseRefName,
		Additions:     rc.Additions,
		Deletions:     rc.Deletions,
		PendingID:     rc.PendingID,
	}
	for _, t := range rc.Threads {
		dc.Threads = append(dc.Threads, toReviewThread(t))
	}
	return dc
}

func toReviewThread(t gql.ReviewThread) domain.ReviewThread {
	rt := domain.ReviewThread{
		Path:     t.Path,
		Line:     t.OriginalLine,
		Resolved: t.IsResolved,
		Outdated: t.IsOutdated,
	}
	if t.Line != nil {
		rt.Line = *t.Line
	}
	if t.DiffSide == "LEFT" {
		rt.Side = domain.SideLeft
	}
	for _, c := range t.Comments.Nodes {
		rt.Comments = append(rt.Comments, toThreadComment(c))
	}
	return rt
}

func toThreadComment(c gql.ThreadComment) domain.ThreadComment {
	return domain.ThreadComment{
		Author:    domain.Author{Login: c.Author.Login},
		Body:      c.Body,
		CreatedAt: c.CreatedAt,
		// PENDING is the only review state that means "written but not
		// sent"; every other one means the comment is already public.
		Pending: c.PullRequestReview.State == "PENDING",
	}
}
