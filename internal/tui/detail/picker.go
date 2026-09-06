package detail

import (
	"strings"

	"github.com/kukv/octoscope/internal/i18n"
	"github.com/kukv/octoscope/internal/tui/layout"
	"github.com/kukv/octoscope/internal/tui/theme"
)

type pickerKind int

const (
	pickLabels pickerKind = iota
	pickAssignees
)

type pickItem struct {
	name     string
	color    string // the label's hex colour; empty for an assignee
	selected bool
}

type picker struct {
	kind     pickerKind
	title    string
	items    []pickItem
	original map[string]bool // what was applied when the picker opened
	cursor   int
	offset   int    // index of the first row in the scroll window
	err      string // the most recent apply failure
}

// newPicker builds a picker whose items are the union of candidates and the
// currently-applied values, with current values pre-selected. Including
// current-but-uncandidate values prevents a hidden item from being removed on
// apply.
func newPicker(kind pickerKind, title string, candidates []string, colors map[string]string, current []string) picker {
	currentSet := make(map[string]bool, len(current))
	for _, c := range current {
		currentSet[c] = true
	}
	seen := make(map[string]bool)
	var items []pickItem
	add := func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		items = append(items, pickItem{name: name, color: colors[name], selected: currentSet[name]})
	}
	for _, c := range candidates {
		add(c)
	}
	for _, c := range current {
		add(c)
	}
	return picker{kind: kind, title: title, items: items, original: currentSet}
}

func (p *picker) toggle() {
	if len(p.items) == 0 {
		return
	}
	p.items[p.cursor].selected = !p.items[p.cursor].selected
}

func (p *picker) moveDown(visible int) {
	if p.cursor < len(p.items)-1 {
		p.cursor++
		if p.cursor >= p.offset+visible {
			p.offset = p.cursor - visible + 1
		}
	}
}

func (p *picker) moveUp() {
	if p.cursor > 0 {
		p.cursor--
		if p.cursor < p.offset {
			p.offset = p.cursor
		}
	}
}

func (p picker) diff() (add, remove []string) {
	for _, it := range p.items {
		switch {
		case it.selected && !p.original[it.name]:
			add = append(add, it.name)
		case !it.selected && p.original[it.name]:
			remove = append(remove, it.name)
		}
	}
	return add, remove
}

func (p picker) listView(height, width int) string {
	var b strings.Builder
	b.WriteString(layout.ClipLines(theme.Title().Render(p.title), width) + "\n\n")
	if len(p.items) == 0 {
		b.WriteString(layout.ClipLines(theme.Dim().Render(i18n.T("picker.no_candidates")), width) + "\n")
	}
	visible := visibleRows(height)
	end := p.offset + visible
	if end > len(p.items) {
		end = len(p.items)
	}
	for i := p.offset; i < end; i++ {
		it := p.items[i]
		box := "[ ]"
		if it.selected {
			box = "[x]"
		}
		name := it.name
		if it.color != "" {
			name = theme.Badge(it.color).Render(" " + name + " ")
		}
		b.WriteString(layout.ClipLines(cursorPrefix(i == p.cursor)+box+" "+name, width) + "\n")
	}
	if p.err != "" {
		b.WriteString("\n" + wrapErr(p.err, width) + "\n")
	}
	return b.String()
}

// visibleRows is how many candidate rows fit given the terminal height.
func visibleRows(height int) int {
	if height <= 0 {
		return 10
	}
	return max(height-6, 3)
}
