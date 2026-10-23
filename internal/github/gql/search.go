package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"
)

//go:embed work.graphql
var workQuery string

type workResponse struct {
	Data struct {
		Results struct {
			Nodes []SearchItem `json:"nodes"`
		} `json:"results"`
	} `json:"data"`
}

// SearchItem is one row of a GitHub issue search, as work.graphql selects it.
type SearchItem struct {
	Typename       string    `json:"__typename"`
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	State          string    `json:"state"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	BodyText       string    `json:"bodyText"`
	UpdatedAt      time.Time `json:"updatedAt"`
	ReviewDecision string    `json:"reviewDecision"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	Author         Author    `json:"author"`
	Repository     struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Labels struct {
		Nodes []Label `json:"nodes"`
	} `json:"labels"`
	Commits lastCommit `json:"commits"`
}

// lastCommit is the commits(last: 1) the list and item documents select:
// GitHub hangs the roll-up off the head commit, not the pull request.
type lastCommit struct {
	Nodes []struct {
		Commit struct {
			StatusCheckRollup *struct {
				Contexts struct {
					TotalCount                 int            `json:"totalCount"`
					CheckRunCountsByState      []StateCount   `json:"checkRunCountsByState"`
					StatusContextCountsByState []StateCount   `json:"statusContextCountsByState"`
					Nodes                      []CheckContext `json:"nodes"`
				} `json:"contexts"`
			} `json:"statusCheckRollup"`
		} `json:"commit"`
	} `json:"nodes"`
}

// CheckRollup is a commit's roll-up. Contexts holds only the first page; the
// counts and TotalCount cover every context, the ones past that page too.
type CheckRollup struct {
	Contexts            []CheckContext
	CheckRunCounts      []StateCount
	StatusContextCounts []StateCount
	TotalCount          int
}

// StateCount is how many contexts are in one state. A CheckRun's state is
// its conclusion once it has completed and its status until then.
type StateCount struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

func (c lastCommit) rollup() CheckRollup {
	var r CheckRollup
	for _, commit := range c.Nodes {
		if rollup := commit.Commit.StatusCheckRollup; rollup != nil {
			r.Contexts = append(r.Contexts, rollup.Contexts.Nodes...)
			r.CheckRunCounts = append(r.CheckRunCounts, rollup.Contexts.CheckRunCountsByState...)
			r.StatusContextCounts = append(r.StatusContextCounts, rollup.Contexts.StatusContextCountsByState...)
			r.TotalCount += rollup.Contexts.TotalCount
		}
	}
	return r
}

type CheckContext struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// SearchItems runs one GitHub issue search and returns what it found. The
// query is the user's, so a rejected one is an ordinary failure to report
// rather than a broken document.
//
// There is no partial body worth salvaging, unlike RepoCounts: a search has
// one result set, and half of one would be read as "that is all there is".
// The error carries what GitHub said, which is what a user has to act on.
func (c *Client) SearchItems(ctx context.Context, search string) ([]SearchItem, error) {
	out, err := c.Read(ctx, workQuery, S("search", search))
	if err != nil {
		return nil, err
	}
	var resp workResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse search: %w", err)
	}
	return resp.Data.Results.Nodes, nil
}

func (n SearchItem) StatusCheckRollup() CheckRollup {
	return n.Commits.rollup()
}
