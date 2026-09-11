package dialog_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/kukv/octoscope/internal/gh"
	"github.com/kukv/octoscope/internal/tui/dialog"
)

func key(s string) tea.KeyPressMsg {
	switch s {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

func typeInto(m dialog.Model, s string) dialog.Model {
	for _, r := range s {
		m, _ = m.Update(key(string(r)))
	}
	return m
}

// The debounce reads Query, so a keystroke that never reaches it would leave
// the suggestions searching for nothing.
func TestTypingReachesTheQuery(t *testing.T) {
	t.Parallel()

	m := typeInto(dialog.New("t", "h"), "koto")
	if m.Query() != "koto" {
		t.Errorf("Query() = %q, want \"koto\"", m.Query())
	}
}

// A suggestion is the only way a name the user never typed can be added, so
// the cursor being on one is what makes it the answer.
func TestValueIsWhatWasTypedUntilACandidateIsPicked(t *testing.T) {
	t.Parallel()

	m := typeInto(dialog.New("t", "h"), "koto")
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto", Stars: 3}})
	if m.Value() != "koto" {
		t.Errorf("Value() = %q before tab, want what was typed", m.Value())
	}
	m, _ = m.Update(key("tab"))
	if m.Value() != "kukv/koto" {
		t.Errorf("Value() = %q after tab, want the candidate", m.Value())
	}
}

func TestMovingBackOffTheCandidatesRestoresWhatWasTyped(t *testing.T) {
	t.Parallel()

	m := typeInto(dialog.New("t", "h"), "koto")
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto"}})
	m, _ = m.Update(key("tab"))
	m, _ = m.Update(key("up"))
	if m.Value() != "koto" {
		t.Errorf("Value() = %q, want what was typed", m.Value())
	}
}

// Suggestions arrive a second after the keystroke that asked for them.
// Landing on one would change what enter adds while the user is still
// typing.
func TestNewCandidatesDoNotMoveTheCursorOffTheField(t *testing.T) {
	t.Parallel()

	m := typeInto(dialog.New("t", "h"), "koto")
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto"}})
	if m.Value() != "koto" {
		t.Errorf("Value() = %q, want the field to keep the cursor", m.Value())
	}
}

// The title says what the popup is for, the hint says what to type, and the
// suggestions are what can be picked instead: all three are what the user
// reads before pressing anything.
func TestViewDrawsTheTitleHintAndCandidates(t *testing.T) {
	t.Parallel()

	m := dialog.New("Add a repository", "Type owner/name").SetWidth(60)
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "kukv/koto", Stars: 12}})
	view := m.View()
	for _, want := range []string{"Add a repository", "Type owner/name", "kukv/koto", "12"} {
		if !strings.Contains(view, want) {
			t.Errorf("view has no %q:\n%s", want, view)
		}
	}
}

// A suggestion row that overruns the box is wrapped by lipgloss, which puts
// the star count on a line of its own and makes the list unreadable. The
// count of border and padding columns is what gets this wrong, so the test
// is the number of lines, not their width.
func TestASuggestionStaysOnOneLine(t *testing.T) {
	t.Parallel()

	for _, width := range []int{80, 120, 160} {
		m := dialog.New("Add a repository", "Type owner/name").SetWidth(width)
		m = m.SetCandidates([]gh.RepoCandidate{
			{Name: "charmbracelet/lipgloss", Stars: 11812},
			{Name: "marcoroth/lipgloss-ruby", Stars: 58},
		})
		lines := strings.Split(m.View(), "\n")
		for _, want := range []string{"11812", "58"} {
			for _, line := range lines {
				text := strings.TrimSpace(ansi.Strip(strings.Trim(ansi.Strip(line), "│")))
				if text == want {
					t.Errorf("at %d columns the star count wrapped onto its own line:\n%s",
						width, ansi.Strip(m.View()))
				}
			}
		}
	}
}

// The suggestions are the whole reason the box is wider than the field, and
// a name cut off at eighty columns cannot be told from its neighbour.
func TestTheBoxFitsAnEightyColumnTerminal(t *testing.T) {
	t.Parallel()

	m := typeInto(dialog.New("Add a repository", "Type owner/name").SetWidth(80), "charmbracelet/lip")
	m = m.SetCandidates([]gh.RepoCandidate{{Name: "charmbracelet/lipgloss", Stars: 11812}})
	for _, line := range strings.Split(m.View(), "\n") {
		if w := ansi.StringWidth(line); w > 80 {
			t.Errorf("a line is %d columns wide: %q", w, line)
		}
	}
	if !strings.Contains(m.View(), "charmbracelet/lipgloss") {
		t.Errorf("the suggestion does not fit at eighty columns:\n%s", m.View())
	}
}
