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

// workSearches is each column's GitHub search. The strings are fixed text,
// not user input: they are the definition of what the column means.
var workSearches = [gh.WorkSectionCount]string{
	gh.SectionReviewRequested: "is:open is:pr review-requested:@me",
	gh.SectionYourPRs:         "is:open is:pr author:@me",
	gh.SectionAssigned:        "is:open assignee:@me",
	gh.SectionMentioned:       "is:open mentions:@me",
}

type workResponse struct {
	Data struct {
		Results struct {
			Nodes []searchNode `json:"nodes"`
		} `json:"results"`
	} `json:"data"`
}

type searchNode struct {
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

// ListWorkSection fetches one column of the Work board. The document itself
// travels as gh's own "query" parameter, so the column's search string has to
// go under a different name.
func (c *Client) ListWorkSection(ctx context.Context, s gh.WorkSection) ([]gh.WorkItem, error) {
	if s < 0 || int(s) >= len(workSearches) {
		return nil, fmt.Errorf("unknown work section %d", s)
	}
	// The search string is fixed text embedded at build time, not data that
	// varies per call (unlike RepoCounts' per-repository names). An "errors"
	// array therefore means the document itself is broken -- there is no
	// partial body worth salvaging, so bail out.
	out, err := c.read(ctx, c.dir, "api", "graphql",
		"-f", "query="+workQuery, "-f", "search="+workSearches[s])
	if err != nil {
		return nil, err
	}
	var resp workResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse work search: %w", err)
	}
	nodes := resp.Data.Results.Nodes
	items := make([]gh.WorkItem, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, n.toWorkItem())
	}
	return items, nil
}

func (n searchNode) toWorkItem() gh.WorkItem {
	item := gh.WorkItem{
		Ref: gh.ItemRef{
			Kind:   gh.ItemIssue,
			Repo:   n.Repository.NameWithOwner,
			Number: n.Number,
		},
		Title:     n.Title,
		State:     gh.ParseItemState(n.State),
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
	item.Head = n.HeadRefName
	item.Base = n.BaseRefName
	item.Additions = n.Additions
	item.Deletions = n.Deletions
	item.Checks = n.checks()
	return item
}

// checks reads the roll-up out of the commit the search returned.
func (n searchNode) checks() gh.Checks {
	var nodes []checkNode
	for _, commit := range n.Commits.Nodes {
		if rollup := commit.Commit.StatusCheckRollup; rollup != nil {
			nodes = append(nodes, rollup.Contexts.Nodes...)
		}
	}
	return rollup(nodes)
}

// rollup counts every check-run context once: each context increments Total
// and exactly one of Passed, Failed, or Running, so Passed+Failed+Running
// always equals Total. It is a free function because `gh pr list` returns the
// same contexts in a flat array, without the commit around them.
func rollup(nodes []checkNode) gh.Checks {
	var c gh.Checks
	for _, node := range nodes {
		c.Total++
		state := checkOutcome(node)
		kind := gh.CheckKindRun
		if node.Typename == "StatusContext" {
			kind = gh.CheckKindStatus
		}
		c.Runs = append(c.Runs, gh.CheckRun{Name: node.name(), State: state, Kind: kind})
		switch state {
		case gh.CheckSuccess:
			c.Passed++
		case gh.CheckFailure:
			c.Failed++
		default:
			c.Running++
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
