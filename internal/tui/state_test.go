package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
)

// runCmds drains a tea.Cmd chain by executing each command and feeding the
// resulting message back through Update, until no more commands are produced.
// This simulates the bubbletea runtime well enough for state-machine tests.
func runCmds(a *App, cmd tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return
		}
		_, cmd = a.Update(msg)
	}
}

func newAppWithReview() *App {
	r := sampleReview()
	r.SummaryFile = "summary.md"
	r.SummaryBody = "# hi"
	return NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})
}

func TestStateStartsOnList(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	if !a.IsList() {
		t.Errorf("expected initial state list, got detail=%v summary=%v", a.IsDetail(), a.IsSummary())
	}
}

func TestEnterTransitionsToDetail(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // summary -> first comment

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runCmds(a, cmd)
	if !a.IsDetail() {
		t.Errorf("expected detail; isList=%v isSummary=%v", a.IsList(), a.IsSummary())
	}
}

func TestEnterOnSummaryRowGoesToSummary(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Default cursor is on the summary row; Enter should land on summary.
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runCmds(a, cmd)
	if !a.IsSummary() {
		t.Errorf("expected summary; isList=%v isDetail=%v", a.IsList(), a.IsDetail())
	}
}

func TestSKeyTransitionsToSummary(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	runCmds(a, cmd)
	if !a.IsSummary() {
		t.Errorf("expected summary; isList=%v isDetail=%v", a.IsList(), a.IsDetail())
	}
}

func TestLKeyReturnsToList(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Enter detail.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // summary -> c1
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runCmds(a, cmd)
	if !a.IsDetail() {
		t.Fatal("setup: expected detail")
	}

	// l → list.
	_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	runCmds(a, cmd)
	if !a.IsList() {
		t.Errorf("expected list after l; isDetail=%v isSummary=%v", a.IsDetail(), a.IsSummary())
	}
}

func TestSummaryCycleEmitsDirty(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Go to summary.
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	runCmds(a, cmd)
	if !a.IsSummary() {
		t.Fatal("setup: expected summary")
	}
	if a.Dirty() {
		t.Fatal("setup: should be clean")
	}

	// c cycles event AND should mark dirty.
	_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	runCmds(a, cmd)
	if !a.Dirty() {
		t.Errorf("c should mark dirty")
	}
}

func TestQuitWorksFromAnyState(t *testing.T) {
	t.Parallel()
	a := newAppWithReview()
	a.Update(tea.WindowSizeMsg{Width: 120, Height: 30})

	// Enter detail.
	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}) // summary -> c1
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	runCmds(a, cmd)

	// Press q from detail; should quit since clean.
	_, cmd = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("got %T, want QuitMsg", cmd())
	}
}
