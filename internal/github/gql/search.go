package gql

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/app/domain"
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

// CheckContext is one entry of a commit's status check rollup. gh pr list
// --json statusCheckRollup returns the same shape in a flat array, which is
// why the roll-up below is a free function rather than a method on the
// commit around it.
type CheckContext struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// name is what the check calls itself. The two shapes spell the field
// differently, so the choice cannot be made by the JSON tags alone.
func (n CheckContext) name() string {
	if n.Typename == "StatusContext" {
		return n.Context
	}
	return n.Name
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

// Checks reads the roll-up out of the commit the search returned.
func (n SearchItem) Checks() domain.Checks {
	var nodes []CheckContext
	for _, commit := range n.Commits.Nodes {
		if rollup := commit.Commit.StatusCheckRollup; rollup != nil {
			nodes = append(nodes, rollup.Contexts.Nodes...)
		}
	}
	return RollupContexts(nodes)
}

// RollupContexts counts every check-run context once: each context
// increments Total and exactly one of Passed, Failed, or Running, so
// Passed+Failed+Running always equals Total.
func RollupContexts(nodes []CheckContext) domain.Checks {
	var c domain.Checks
	for _, node := range nodes {
		c.Total++
		state := checkOutcome(node)
		kind := domain.CheckKindRun
		if node.Typename == "StatusContext" {
			kind = domain.CheckKindStatus
		}
		c.Runs = append(c.Runs, domain.CheckRun{Name: node.name(), State: state, Kind: kind})
		switch state {
		case domain.CheckSuccess:
			c.Passed++
		case domain.CheckFailure:
			c.Failed++
		default:
			c.Running++
		}
	}
	switch {
	case c.Total == 0:
		c.State = domain.CheckNone
	case c.Failed > 0:
		c.State = domain.CheckFailure
	case c.Running > 0:
		c.State = domain.CheckRunning
	default:
		c.State = domain.CheckSuccess
	}
	return c
}

// checkOutcome reads one context of the rollup. CheckRun reports status and
// conclusion; the older StatusContext reports a single state, so the two
// shapes have to be read differently.
func checkOutcome(n CheckContext) domain.CheckState {
	if n.Typename == "StatusContext" {
		switch n.State {
		case "SUCCESS":
			return domain.CheckSuccess
		case "FAILURE", "ERROR":
			return domain.CheckFailure
		default:
			return domain.CheckPending
		}
	}
	if n.Status != "COMPLETED" {
		return domain.CheckRunning
	}
	switch n.Conclusion {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return domain.CheckSuccess
	case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return domain.CheckFailure
	default:
		return domain.CheckPending
	}
}
