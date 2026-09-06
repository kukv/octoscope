package cli

import (
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

// prJSON and issueJSON are the shapes `gh pr view` and `gh issue view` return.
// They exist so that GitHub's own spelling of a state ("OPEN", "MERGED",
// "CHANGES_REQUESTED") stops here: gh.PR and gh.Issue carry the domain values
// instead, and no view has to know how the API writes them
// (.claude/rules/architecture.md).
type prJSON struct {
	Number         int          `json:"number"`
	Title          string       `json:"title"`
	Author         gh.Author    `json:"author"`
	State          string       `json:"state"`
	IsDraft        bool         `json:"isDraft"`
	UpdatedAt      time.Time    `json:"updatedAt"`
	ReviewDecision string       `json:"reviewDecision"`
	URL            string       `json:"url"`
	Body           string       `json:"body"`
	Comments       []gh.Comment `json:"comments"`
	Labels         []gh.Label   `json:"labels"`
	Assignees      []gh.Author  `json:"assignees"`
	HeadRefName    string       `json:"headRefName"`
	BaseRefName    string       `json:"baseRefName"`
	Additions      int          `json:"additions"`
	Deletions      int          `json:"deletions"`
	// gh pr list returns the roll-up as a flat array of contexts, without the
	// commit the GraphQL search nests it under.
	StatusCheckRollup []checkNode `json:"statusCheckRollup"`
}

func (p prJSON) toDomain() gh.PR {
	return gh.PR{
		Number:    p.Number,
		Title:     p.Title,
		Author:    p.Author,
		State:     gh.ParseItemState(p.State),
		IsDraft:   p.IsDraft,
		UpdatedAt: p.UpdatedAt,
		Review:    gh.ParseReviewDecision(p.ReviewDecision),
		URL:       p.URL,
		Body:      p.Body,
		Comments:  p.Comments,
		Labels:    p.Labels,
		Assignees: p.Assignees,
		Head:      p.HeadRefName,
		Base:      p.BaseRefName,
		Additions: p.Additions,
		Deletions: p.Deletions,
		Checks:    rollup(p.StatusCheckRollup),
	}
}

func toPRs(in []prJSON) []gh.PR {
	out := make([]gh.PR, len(in))
	for i, p := range in {
		out[i] = p.toDomain()
	}
	return out
}

type issueJSON struct {
	Number    int          `json:"number"`
	Title     string       `json:"title"`
	Author    gh.Author    `json:"author"`
	State     string       `json:"state"`
	UpdatedAt time.Time    `json:"updatedAt"`
	URL       string       `json:"url"`
	Body      string       `json:"body"`
	Comments  []gh.Comment `json:"comments"`
	Labels    []gh.Label   `json:"labels"`
	Assignees []gh.Author  `json:"assignees"`
}

func (i issueJSON) toDomain() gh.Issue {
	return gh.Issue{
		Number:    i.Number,
		Title:     i.Title,
		Author:    i.Author,
		State:     gh.ParseItemState(i.State),
		UpdatedAt: i.UpdatedAt,
		URL:       i.URL,
		Body:      i.Body,
		Comments:  i.Comments,
		Labels:    i.Labels,
		Assignees: i.Assignees,
	}
}

func toIssues(in []issueJSON) []gh.Issue {
	out := make([]gh.Issue, len(in))
	for i, v := range in {
		out[i] = v.toDomain()
	}
	return out
}
