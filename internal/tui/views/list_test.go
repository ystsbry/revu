package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

func sampleReview() *model.Review {
	return &model.Review{
		SchemaVersion: 1,
		PR:            model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "abc1234", BaseBranch: "main"},
		ReviewEvent:   model.EventComment,
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor, Category: model.CategoryDesign, Path: "a/x.go", Line: 10, Side: model.SideRight, BodyFile: "a.md", Body: "x"},
			{ID: "c2", Status: model.StatusPending, Severity: model.SeverityNit, Category: model.CategoryStyle, Path: "a/y.go", Line: 20, Side: model.SideRight, BodyFile: "b.md", Body: "y"},
		},
	}
}

func TestListAcceptKeyMutates(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l := NewList(r, keys.DefaultKeyMap())

	got, cmd := l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if got != l {
		t.Fatalf("Update should return same model")
	}
	if r.Comments[0].Status != model.StatusAccepted {
		t.Errorf("c1 status = %s, want accepted", r.Comments[0].Status)
	}
	if r.Comments[1].Status != model.StatusPending {
		t.Errorf("c2 should remain pending, got %s", r.Comments[1].Status)
	}
	if cmd == nil {
		t.Fatal("expected DirtyMsg cmd")
	}
	if _, ok := cmd().(DirtyMsg); !ok {
		t.Fatalf("cmd produced %T, want DirtyMsg", cmd())
	}
}

func TestListRejectAndPendingKeys(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l := NewList(r, keys.DefaultKeyMap())

	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if r.Comments[0].Status != model.StatusRejected {
		t.Errorf("after r: status = %s", r.Comments[0].Status)
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if r.Comments[0].Status != model.StatusPending {
		t.Errorf("after u: status = %s", r.Comments[0].Status)
	}
}

func TestListFooterShowsCounts(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	r.Comments[0].Status = model.StatusAccepted
	l := NewList(r, keys.DefaultKeyMap())
	view := l.View()
	if !strings.Contains(view, "Accepted: 1") {
		t.Errorf("expected 'Accepted: 1' in view, got:\n%s", view)
	}
	if !strings.Contains(view, "Pending: 1") {
		t.Errorf("expected 'Pending: 1' in view, got:\n%s", view)
	}
}
