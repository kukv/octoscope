// Package dialog is the popup that asks for one name, offering suggestions
// while it is typed into. It draws its own box and fetches nothing; the
// holder places it and answers for it.
package dialog

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/kukv/octoscope/internal/gh"
)

type Model struct {
	title, hint string
	input       textinput.Model
	candidates  []gh.RepoCandidate

	// cursor is -1 while the field has the focus and indexes candidates
	// otherwise. There is no third place to be, so it doubles as the focus.
	cursor int

	width     int
	errText   string
	searching bool
}

// New returns a dialog with the field focused and no suggestions yet.
func New(title, hint string) Model {
	in := textinput.New()
	in.Focus()
	return Model{title: title, hint: hint, input: in, cursor: -1}
}

// Query is what has been typed, which is what a search is run for.
func (m Model) Query() string { return m.input.Value() }

// Value is the name enter would take: the suggestion under the cursor, or
// what was typed while the field still has it.
func (m Model) Value() string {
	if c, ok := m.candidate(); ok {
		return c.Name
	}
	return m.input.Value()
}

func (m Model) candidate() (gh.RepoCandidate, bool) {
	if m.cursor < 0 || m.cursor >= len(m.candidates) {
		return gh.RepoCandidate{}, false
	}
	return m.candidates[m.cursor], true
}

// SetCandidates replaces the suggestions without moving the cursor onto
// them: they arrive a second after the keystroke that asked for them, and
// landing on one would change what enter takes mid-sentence.
func (m Model) SetCandidates(c []gh.RepoCandidate) Model {
	m.candidates = c
	m.searching = false
	if m.cursor >= len(c) {
		m.cursor = len(c) - 1
	}
	return m
}

// SetWidth fits the box and the field to the terminal.
func (m Model) SetWidth(w int) Model {
	m.width = w
	m.input.SetWidth(max(m.contentWidth()-promptCols, 1))
	return m
}

// SetError puts one line under the suggestions. Typing clears it.
func (m Model) SetError(text string) Model { m.errText = text; return m }

// SetValue starts the field with v already in it.
func (m Model) SetValue(v string) Model {
	m.input.SetValue(v)
	return m
}

// Searching marks that a request is in flight, which the box says while the
// suggestions are still blank.
func (m Model) Searching() Model { m.searching = true; return m }

// Update handles the keys that move between the field and the suggestions,
// and hands everything else to the field. enter and esc belong to the
// holder: it is what adds and what closes.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	switch key.String() {
	case "tab", "down":
		if m.cursor < len(m.candidates)-1 {
			m.cursor++
		}
		return m, nil
	case "shift+tab", "up":
		if m.cursor >= 0 {
			m.cursor--
		}
		return m, nil
	}
	m.errText = ""
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}
