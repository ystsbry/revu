package views

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

func summaryFixture() *model.Review {
	return &model.Review{
		BaseDir:     "/tmp/fake",
		PR:          model.PRMeta{Repo: "o/r", Number: 1},
		ReviewEvent: model.EventComment,
		SummaryFile: "summary.md",
		SummaryBody: "# Header\n\nbody text\n",
	}
}

func TestSummaryCycleEvent(t *testing.T) {
	t.Parallel()
	r := summaryFixture()
	s := NewSummary(r, keys.DefaultKeyMap())

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if r.ReviewEvent != model.EventRequestChanges {
		t.Errorf("after first c: %s", r.ReviewEvent)
	}
	if cmd == nil || cmd() == nil {
		t.Errorf("c should emit DirtyMsg")
	}

	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if r.ReviewEvent != model.EventApprove {
		t.Errorf("after second c: %s", r.ReviewEvent)
	}
	s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if r.ReviewEvent != model.EventComment {
		t.Errorf("after third c: %s", r.ReviewEvent)
	}
}

func TestSummaryGoToList(t *testing.T) {
	t.Parallel()
	r := summaryFixture()
	s := NewSummary(r, keys.DefaultKeyMap())

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	if cmd == nil {
		t.Fatal("expected GoToListMsg")
	}
	if _, ok := cmd().(GoToListMsg); !ok {
		t.Errorf("got %T, want GoToListMsg", cmd())
	}
}

func TestSummaryEditEmitsAbsPath(t *testing.T) {
	t.Parallel()
	r := summaryFixture()
	s := NewSummary(r, keys.DefaultKeyMap())

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if cmd == nil {
		t.Fatal("expected EditMsg")
	}
	em, ok := cmd().(EditMsg)
	if !ok {
		t.Fatalf("got %T", cmd())
	}
	want := filepath.Join("/tmp/fake", "summary.md")
	if em.Path != want {
		t.Errorf("EditMsg.Path = %q, want %q", em.Path, want)
	}
}

func TestSummaryViewShowsRadio(t *testing.T) {
	t.Parallel()
	r := summaryFixture()
	s := NewSummary(r, keys.DefaultKeyMap())
	s.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := s.View()
	if !strings.Contains(view, "(●) COMMENT") {
		t.Errorf("expected (●) COMMENT marker; view:\n%s", view)
	}
}
