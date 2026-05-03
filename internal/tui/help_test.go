package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpToggleAndDismiss(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	if a.showHelp {
		t.Fatal("help should start hidden")
	}
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !a.showHelp {
		t.Errorf("? should show help")
	}
	if !strings.Contains(a.View(), "keybindings") {
		t.Errorf("help view should mention 'keybindings':\n%s", a.View())
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if a.showHelp {
		t.Errorf("Esc should hide help")
	}
}

func TestHelpSwallowsKeys(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !a.showHelp {
		t.Fatal("setup")
	}

	// 'a' would normally accept; while help is up it must do nothing.
	r := a.review
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	for _, c := range r.Comments {
		if c.Status != "pending" {
			t.Errorf("comment %s status leaked through help: %s", c.ID, c.Status)
		}
	}
}
