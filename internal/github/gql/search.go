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
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []CheckContext `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

// CheckContext is one entry of a commit's status check rollup.
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

// StatusCheckContexts reads the roll-up out of the commit the search
// returned.
func (n SearchItem) StatusCheckContexts() []CheckContext {
	var nodes []CheckContext
	for _, commit := range n.Commits.Nodes {
		if rollup := commit.Commit.StatusCheckRollup; rollup != nil {
			nodes = append(nodes, rollup.Contexts.Nodes...)
		}
	}
	return nodes
}
