package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/filter"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

func filterFixture() *model.Review {
	return &model.Review{
		PR:          model.PRMeta{Repo: "o/r", Number: 1, HeadSHA: "abc1234"},
		ReviewEvent: model.EventComment,
		SummaryBody: "# hi",
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor, Category: model.CategoryDesign, Path: "a.go", Line: 1, Side: model.SideRight, Body: "x"},
			{ID: "c2", Status: model.StatusPending, Severity: model.SeverityNit, Category: model.CategoryStyle, Path: "b.go", Line: 2, Side: model.SideRight, Body: "y"},
			{ID: "c3", Status: model.StatusAccepted, Severity: model.SeverityMajor, Category: model.CategoryBug, Path: "c.go", Line: 3, Side: model.SideRight, Body: "z"},
		},
	}
}

func TestSetFilterJumpsToFirstVisible(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	if l.Cursor() != -1 {
		t.Fatalf("setup: cursor = %d", l.Cursor())
	}
	f, _ := filter.Parse("severity:major")
	l.SetFilter(f)
	// First visible is c1 (index 0). Per judgment 3b: cursor jumps there.
	if l.Cursor() != 0 {
		t.Errorf("cursor after filter = %d, want 0 (c1)", l.Cursor())
	}
	if l.VisibleCount() != 2 {
		t.Errorf("visible = %d, want 2", l.VisibleCount())
	}
}

func TestSetFilterEmptyResultLandsOnSummary(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	f, _ := filter.Parse("severity:critical")
	l.SetFilter(f)
	if l.Cursor() != -1 {
		t.Errorf("empty result: cursor = %d, want -1", l.Cursor())
	}
	if l.VisibleCount() != 0 {
		t.Errorf("VisibleCount = %d, want 0", l.VisibleCount())
	}
}

func TestNavigationAfterFilter(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	f, _ := filter.Parse("severity:major") // c1, c3
	l.SetFilter(f)
	if l.Cursor() != 0 { // c1
		t.Fatalf("setup: cursor = %d", l.Cursor())
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if l.Cursor() != 2 { // c3 (skipping c2)
		t.Errorf("after j: cursor = %d, want 2", l.Cursor())
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if l.Cursor() != 2 {
		t.Errorf("clamps at last visible: %d", l.Cursor())
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if l.Cursor() != 0 {
		t.Errorf("after k: %d", l.Cursor())
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if l.Cursor() != -1 {
		t.Errorf("up from first visible should hit summary, got %d", l.Cursor())
	}
}

func TestSlashEntersFilterMode(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	if l.IsFilterMode() {
		t.Fatal("should not start in filter mode")
	}
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if !l.IsFilterMode() {
		t.Errorf("/ should enter filter mode")
	}
}

func TestFilterModeEscCancels(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, c := range "severity:major" {
		l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
	}
	l.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if l.IsFilterMode() {
		t.Errorf("Esc should exit filter mode")
	}
	if !l.filter.IsEmpty() {
		t.Errorf("Esc should not apply filter; got %q", l.filter.String())
	}
}

func TestFilterModeEnterApplies(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, c := range "severity:major" {
		l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
	}
	_, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil && cmd() != nil {
		// Successful parse should produce no message (filter applied locally).
		if _, ok := cmd().(FilterErrMsg); ok {
			t.Errorf("got FilterErrMsg for valid expr")
		}
	}
	if l.IsFilterMode() {
		t.Errorf("Enter should exit filter mode")
	}
	if l.VisibleCount() != 2 {
		t.Errorf("visible = %d, want 2", l.VisibleCount())
	}
}

func TestFilterModeBadExprEmitsErrMsg(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	for _, c := range "severity:zzz" {
		l.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
	}
	_, cmd := l.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected FilterErrMsg")
	}
	msg := cmd()
	if _, ok := msg.(FilterErrMsg); !ok {
		t.Errorf("got %T, want FilterErrMsg", msg)
	}
}

func TestFilterStatusInView(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	l.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	f, _ := filter.Parse("severity:major")
	l.SetFilter(f)
	v := l.View()
	if !strings.Contains(v, "Filter:") {
		t.Errorf("view should show filter status:\n%s", v)
	}
	if !strings.Contains(v, "Showing: 2 of 3") {
		t.Errorf("view should show visibility ratio:\n%s", v)
	}
}

func TestClearFilterRestoresAll(t *testing.T) {
	t.Parallel()
	r := filterFixture()
	l := NewList(r, keys.DefaultKeyMap())
	f, _ := filter.Parse("severity:major")
	l.SetFilter(f)
	l.ClearFilter()
	if l.VisibleCount() != 3 {
		t.Errorf("after clear: visible = %d, want 3", l.VisibleCount())
	}
}
