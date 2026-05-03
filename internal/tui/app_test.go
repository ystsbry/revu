package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
)

func sampleReview() *model.Review {
	return &model.Review{
		SchemaVersion: 1,
		PR:            model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "abc1234", BaseBranch: "main"},
		ReviewEvent:   model.EventComment,
		BaseDir:       "/tmp/fake",
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor, Category: model.CategoryDesign, Path: "a/x.go", Line: 10, Side: model.SideRight, BodyFile: "a.md"},
			{ID: "c2", Status: model.StatusPending, Severity: model.SeverityNit, Category: model.CategoryStyle, Path: "a/y.go", Line: 20, Side: model.SideRight, BodyFile: "b.md"},
		},
	}
}

// driveKey simulates a sequence of key presses through the app's Update loop,
// invoking each returned tea.Cmd once so DirtyMsg etc. are processed.
func driveKey(a *App, key tea.KeyMsg) tea.Cmd {
	model, cmd := a.Update(key)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			model.Update(msg)
		}
	}
	_ = model
	return cmd
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestAppDirtyAfterAccept(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	if a.Dirty() {
		t.Fatalf("should start clean")
	}
	a.Update(runeKey('j')) // summary -> c1
	driveKey(a, runeKey('a'))
	if !a.Dirty() {
		t.Errorf("expected dirty after 'a' key")
	}
	if r.Comments[0].Status != model.StatusAccepted {
		t.Errorf("status mismatch: %s", r.Comments[0].Status)
	}
}

func TestAppSaveCommand(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	saved := false
	a := NewApp(Config{Review: r, Saver: func(rr *model.Review) error { saved = true; return nil }})

	a.Update(runeKey('j'))    // summary -> c1
	driveKey(a, runeKey('a')) // mutate, dirty=true
	if !a.Dirty() {
		t.Fatal("should be dirty")
	}

	// :save<Enter>
	a.Update(runeKey(':'))
	for _, c := range "save" {
		a.Update(runeKey(c))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !saved {
		t.Errorf("saver was not invoked")
	}
	if a.Dirty() {
		t.Errorf("should be clean after save")
	}
	if !strings.Contains(a.View(), "saved") {
		t.Errorf("expected 'saved' status; view:\n%s", a.View())
	}
}

func TestAppSaveErrorKeepsDirty(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return errors.New("boom") }})

	a.Update(runeKey('j')) // summary -> c1
	driveKey(a, runeKey('a'))

	a.Update(runeKey(':'))
	for _, c := range "save" {
		a.Update(runeKey(c))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !a.Dirty() {
		t.Errorf("should remain dirty after save failure")
	}
	if !strings.Contains(a.View(), "save failed") {
		t.Errorf("view should mention save failure:\n%s", a.View())
	}
}

func TestAppQuitGuardsAgainstDirty(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	a.Update(runeKey('j'))    // summary -> c1
	driveKey(a, runeKey('a')) // dirty

	_, cmd := a.Update(runeKey('q'))
	if cmd != nil {
		// First q with dirty must not produce tea.Quit.
		if msg, ok := cmd().(tea.QuitMsg); ok {
			t.Fatalf("first 'q' on dirty should not quit, got QuitMsg: %#v", msg)
		}
	}
	if !strings.Contains(a.View(), "unsaved") {
		t.Errorf("expected unsaved warning; view:\n%s", a.View())
	}

	// Second q proceeds.
	_, cmd = a.Update(runeKey('q'))
	if cmd == nil {
		t.Fatal("second 'q' should return a quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("second 'q' on dirty should produce QuitMsg, got %T", cmd())
	}
}

func TestAppQuitClean(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	_, cmd := a.Update(runeKey('q'))
	if cmd == nil {
		t.Fatal("expected quit cmd")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("clean 'q' should QuitMsg, got %T", cmd())
	}
}

func TestAppForceQuitCommand(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	a.Update(runeKey('j'))    // summary -> c1
	driveKey(a, runeKey('a')) // dirty

	a.Update(runeKey(':'))
	for _, c := range "q!" {
		a.Update(runeKey(c))
	}
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected quit cmd from :q!")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf(":q! should QuitMsg even when dirty, got %T", cmd())
	}
}

func TestAppUnknownCommand(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	a := NewApp(Config{Review: r, Saver: func(*model.Review) error { return nil }})

	a.Update(runeKey(':'))
	for _, c := range "bogus" {
		a.Update(runeKey(c))
	}
	a.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !strings.Contains(a.View(), "unknown command") {
		t.Errorf("expected unknown command error; view:\n%s", a.View())
	}
}
