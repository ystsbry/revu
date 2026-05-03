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

// pressJ moves the list cursor down one (summary -> first comment, etc.).
func pressJ(l *List) {
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
}

func TestListAcceptKeyMutates(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l := NewList(r, keys.DefaultKeyMap())
	pressJ(l) // land on c1

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
	pressJ(l)

	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	if r.Comments[0].Status != model.StatusRejected {
		t.Errorf("after r: status = %s", r.Comments[0].Status)
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'u'}})
	if r.Comments[0].Status != model.StatusPending {
		t.Errorf("after u: status = %s", r.Comments[0].Status)
	}
}

func TestListSummarySelectedByDefault(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l := NewList(r, keys.DefaultKeyMap())
	if l.Cursor() != -1 {
		t.Errorf("initial cursor = %d, want -1 (summary)", l.Cursor())
	}
}

func TestListEnterOnSummaryEmitsSummaryMsg(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	r.SummaryBody = "# header\n\nbody"
	l := NewList(r, keys.DefaultKeyMap())
	_, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected GoToSummaryMsg cmd")
	}
	if _, ok := cmd().(GoToSummaryMsg); !ok {
		t.Fatalf("got %T, want GoToSummaryMsg", cmd())
	}
}

func TestListEnterOnCommentEmitsDetailMsg(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l := NewList(r, keys.DefaultKeyMap())
	pressJ(l) // cursor -> c1

	_, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected GoToDetailMsg cmd")
	}
	dm, ok := cmd().(GoToDetailMsg)
	if !ok {
		t.Fatalf("got %T, want GoToDetailMsg", cmd())
	}
	if dm.Index != 0 {
		t.Errorf("Index = %d, want 0", dm.Index)
	}
}

func TestListNavigationWrapping(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l := NewList(r, keys.DefaultKeyMap())

	// Up from summary stays on summary.
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if l.Cursor() != -1 {
		t.Errorf("up from -1 = %d", l.Cursor())
	}
	// Down through all comments then clamps at last.
	pressJ(l)
	pressJ(l)
	pressJ(l)
	pressJ(l)
	if l.Cursor() != 1 { // sampleReview has 2 comments
		t.Errorf("down past end: cursor = %d, want 1", l.Cursor())
	}
	// Up from first comment returns to summary.
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if l.Cursor() != -1 {
		t.Errorf("up from c1 should return to summary, got %d", l.Cursor())
	}
}

func TestListAcceptOnSummaryIsNoOp(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l := NewList(r, keys.DefaultKeyMap())

	_, cmd := l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	for _, c := range r.Comments {
		if c.Status != model.StatusPending {
			t.Errorf("comment %s status changed: %s", c.ID, c.Status)
		}
	}
	if cmd != nil {
		t.Errorf("'a' on summary should not emit cmd, got %T", cmd())
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
