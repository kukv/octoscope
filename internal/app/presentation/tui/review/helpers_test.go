package review

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// key builds the KeyPressMsg for a key name, matching the shape the app uses
// (internal/tui/diff/diff_test.go's key, with a tab case this package needs).
func key(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "ctrl+s":
		return tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
	}
}

// keyPress is key's name in the brief; both build the same KeyPressMsg.
func keyPress(s string) tea.KeyMsg { return key(s) }

// typeInto sends one KeyPressMsg per rune, the way a user typing into the
// composer would.
func typeInto(m Model, s string) Model {
	for _, r := range s {
		m, _ = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// runCmd runs a command purely for its side effects (what it sends to src),
// without feeding the message it returns back into the model.
func runCmd(t *testing.T, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("no command to run")
	}
	cmd()
}
