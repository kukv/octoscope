// Package search is the tab that searches GitHub from filters the user
// builds, with the query it built on show above them.
package search

import (
	"strings"
)

// FilterID names one row of the filter pane, in the order they are drawn.
type FilterID int

const (
	FilterType FilterID = iota
	FilterState
	FilterOrg
	FilterRepo
	FilterAuthor
	FilterLabel
	FilterReview
	FilterSort
	filterCount
)

// choices is what a picked filter offers, in the order space walks them. A
// filter with no choices is typed into instead. The first entry is the
// default, and an empty first entry means the filter adds nothing until it
// is moved off it.
var choices = [filterCount][]string{
	FilterType:   {"all", "pr", "issue"},
	FilterState:  {"open", "closed", "all"},
	FilterReview: {"", "none", "required", "approved", "changes-requested"},
	FilterSort:   {"", "updated", "created", "comments"},
}

// Choices is what space walks through for a picked filter, and nil for one
// that is typed into.
func (id FilterID) Choices() []string { return choices[id] }

// Filters is the state of the filter pane: which choice each picked filter
// is on, and what was typed into each of the others.
type Filters struct {
	picked [filterCount]int
	typed  [filterCount]string
}

// Value is what the pane draws for one filter, and what the query is built
// from. It is empty when the filter is adding nothing.
func (f Filters) Value(id FilterID) string {
	if c := choices[id]; c != nil {
		return c[f.picked[id]]
	}
	return f.typed[id]
}

// Cycle moves a picked filter onto its next choice and comes back around at
// the end. A typed filter has nothing to cycle through and is left alone.
func (f Filters) Cycle(id FilterID) Filters {
	c := choices[id]
	if c == nil {
		return f
	}
	f.picked[id] = (f.picked[id] + 1) % len(c)
	return f
}

// Set puts v into a typed filter. A picked one is left alone: its values are
// the only ones GitHub understands for it.
func (f Filters) Set(id FilterID, v string) Filters {
	if choices[id] != nil {
		return f
	}
	f.typed[id] = strings.TrimSpace(v)
	return f
}

// Query is the GitHub search these filters stand for. The qualifiers are not
// translated: they are GitHub's own syntax (.claude/rules/tui.md).
func (f Filters) Query() string {
	var parts []string
	switch f.Value(FilterState) {
	case "open":
		parts = append(parts, "is:open")
	case "closed":
		parts = append(parts, "is:closed")
	}
	switch f.Value(FilterType) {
	case "pr":
		parts = append(parts, "is:pr")
	case "issue":
		parts = append(parts, "is:issue")
	}
	for _, q := range []struct {
		id     FilterID
		prefix string
	}{
		{FilterOrg, "org:"},
		{FilterRepo, "repo:"},
		{FilterAuthor, "author:"},
		{FilterLabel, "label:"},
		{FilterReview, "review:"},
		{FilterSort, "sort:"},
	} {
		if v := f.Value(q.id); v != "" {
			parts = append(parts, q.prefix+quote(v))
		}
	}
	return strings.Join(parts, " ")
}

// quote wraps a value that carries a space, which GitHub would otherwise
// read as the end of the qualifier.
func quote(v string) string {
	if !strings.Contains(v, " ") {
		return v
	}
	return `"` + v + `"`
}
