package cli

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kukv/octoscope/internal/gh"
)

//go:embed work.graphql
var workQuery string

// workSearches maps each board column to the GraphQL alias that carries it
// in work.graphql. The query text lives in that file; nothing here builds
// it, so there is no string to keep escaped.
var workSearches = []struct {
	section gh.WorkSection
	alias   string
}{
	{gh.SectionReviewRequested, "reviewRequested"},
	{gh.SectionYourPRs, "yourPRs"},
	{gh.SectionAssigned, "assigned"},
	{gh.SectionMentioned, "mentioned"},
}

type workResponse struct {
	Data map[string]struct {
		Nodes []searchNode `json:"nodes"`
	} `json:"data"`
}

type searchNode struct {
	Typename       string    `json:"__typename"`
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	IsDraft        bool      `json:"isDraft"`
	BodyText       string    `json:"bodyText"`
	UpdatedAt      time.Time `json:"updatedAt"`
	ReviewDecision string    `json:"reviewDecision"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Labels struct {
		Nodes []gh.Label `json:"nodes"`
	} `json:"labels"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []checkNode `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type checkNode struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Context    string `json:"context"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	State      string `json:"state"`
}

// name is what the check calls itself. The two shapes spell the field
// differently, so the choice cannot be made by the JSON tags alone.
func (n checkNode) name() string {
	if n.Typename == "StatusContext" {
		return n.Context
	}
	return n.Name
}

// ListWork fetches every column of the Work board in one GraphQL request.
func (c *Client) ListWork(ctx context.Context) (gh.Work, error) {
	// gh api graphql exits non-zero when the response body carries a top-level
	// "errors" array, so a query GitHub rejects arrives here as an error from
	// c.run rather than as a body we'd otherwise parse into empty columns.
	out, err := c.run(ctx, c.dir, "api", "graphql", "-f", "query="+workQuery)
	if err != nil {
		return gh.Work{}, err
	}
	var resp workResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return gh.Work{}, fmt.Errorf("parse work search: %w", err)
	}
	var w gh.Work
	for _, s := range workSearches {
		nodes := resp.Data[s.alias].Nodes
		items := make([]gh.WorkItem, 0, len(nodes))
		for _, n := range nodes {
			items = append(items, n.toWorkItem())
		}
		w[s.section] = items
	}
	return w, nil
}

func (n searchNode) toWorkItem() gh.WorkItem {
	item := gh.WorkItem{
		Ref: gh.ItemRef{
			Kind:   gh.ItemIssue,
			Repo:   n.Repository.NameWithOwner,
			Number: n.Number,
		},
		Title:     n.Title,
		Body:      n.BodyText,
		Author:    n.Author.Login,
		Labels:    n.Labels.Nodes,
		UpdatedAt: n.UpdatedAt,
		URL:       n.URL,
	}
	if n.Typename != "PullRequest" {
		return item
	}
	item.Ref.Kind = gh.ItemPR
	item.IsDraft = n.IsDraft
	item.Review = gh.ParseReviewDecision(n.ReviewDecision)
	item.Checks = n.checks()
	return item
}

// checks counts every check-run context once: each context increments Total
// and exactly one of Passed, Failed, or Running, so Passed+Failed+Running
// always equals Total.
func (n searchNode) checks() gh.Checks {
	var c gh.Checks
	for _, commit := range n.Commits.Nodes {
		rollup := commit.Commit.StatusCheckRollup
		if rollup == nil {
			continue
		}
		for _, node := range rollup.Contexts.Nodes {
			c.Total++
			state := checkOutcome(node)
			c.Runs = append(c.Runs, gh.CheckRun{Name: node.name(), State: state})
			switch state {
			case gh.CheckSuccess:
				c.Passed++
			case gh.CheckFailure:
				c.Failed++
			default:
				c.Running++
			}
		}
	}
	switch {
	case c.Total == 0:
		c.State = gh.CheckNone
	case c.Failed > 0:
		c.State = gh.CheckFailure
	case c.Running > 0:
		c.State = gh.CheckRunning
	default:
		c.State = gh.CheckSuccess
	}
	return c
}

// checkOutcome reads one context of the rollup. CheckRun reports status and
// conclusion; the older StatusContext reports a single state, so the two
// shapes have to be read differently.
func checkOutcome(n checkNode) gh.CheckState {
	if n.Typename == "StatusContext" {
		switch n.State {
		case "SUCCESS":
			return gh.CheckSuccess
		case "FAILURE", "ERROR":
			return gh.CheckFailure
		default:
			return gh.CheckPending
		}
	}
	if n.Status != "COMPLETED" {
		return gh.CheckRunning
	}
	switch n.Conclusion {
	case "SUCCESS", "NEUTRAL", "SKIPPED":
		return gh.CheckSuccess
	case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return gh.CheckFailure
	default:
		return gh.CheckPending
	}
}
