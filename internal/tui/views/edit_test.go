package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

func editFixture() *model.Review {
	return &model.Review{
		PR: model.PRMeta{Repo: "o/r", Number: 1},
		Comments: []model.Comment{
			{ID: "c1", Status: model.StatusPending, Severity: model.SeverityMajor, Category: model.CategoryDesign, Path: "x.go", Line: 10, Side: model.SideRight},
			{ID: "c2", Status: model.StatusPending, Severity: model.SeverityNit, Category: model.CategoryStyle, Path: "y.go", Line: 20, Side: model.SideRight},
		},
	}
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// gotoLineField presses 'j' enough times to reach the line field from the
// initial severity field. Centralised so tests don't bake in the field count.
func gotoLineField(e *Edit) {
	for e.Field() != fieldLine {
		e.Update(keyRune('j'))
	}
}

func gotoStartLineField(e *Edit) {
	for e.Field() != fieldStartLine {
		e.Update(keyRune('j'))
	}
}

func TestEditNavigateFields(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)

	if e.Field() != fieldSeverity {
		t.Fatalf("initial field = %v, want fieldSeverity", e.Field())
	}
	e.Update(keyRune('j'))
	if e.Field() != fieldCategory {
		t.Errorf("after j: field = %v, want fieldCategory", e.Field())
	}
	e.Update(keyRune('j'))
	if e.Field() != fieldStartLine {
		t.Errorf("after jj: field = %v, want fieldStartLine", e.Field())
	}
	e.Update(keyRune('j'))
	if e.Field() != fieldLine {
		t.Errorf("after jjj: field = %v, want fieldLine", e.Field())
	}
	// Past the last row clamps.
	e.Update(keyRune('j'))
	if e.Field() != fieldLine {
		t.Errorf("clamps at fieldLine, got %v", e.Field())
	}
	for i := 0; i < int(editFieldCount); i++ {
		e.Update(keyRune('k'))
	}
	if e.Field() != fieldSeverity {
		t.Errorf("after k*N: field = %v, want fieldSeverity", e.Field())
	}
}

func TestEditCycleSeverityAndCategory(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)

	defs := model.ActiveSeverityRegistry().All()
	startIdx := -1
	for i, d := range defs {
		if d.Name == string(model.SeverityMajor) {
			startIdx = i
			break
		}
	}
	if startIdx < 0 {
		t.Fatal("major not in default registry")
	}
	wantNext := defs[(startIdx+1)%len(defs)].Name

	_, cmd := e.Update(keyRune('l'))
	if cmd == nil {
		t.Error("expected DirtyMsg cmd from severity cycle")
	}
	if string(r.Comments[0].Severity) != wantNext {
		t.Errorf("severity = %q, want %q", r.Comments[0].Severity, wantNext)
	}

	e.Update(keyRune('j'))
	startCat := r.Comments[0].Category
	_, cmd = e.Update(keyRune('l'))
	if cmd == nil {
		t.Error("expected DirtyMsg cmd from category cycle")
	}
	if r.Comments[0].Category == startCat {
		t.Errorf("category did not change from %q", startCat)
	}

	_, cmd = e.Update(keyRune('h'))
	if cmd == nil {
		t.Error("expected DirtyMsg cmd from category back-cycle")
	}
	if r.Comments[0].Category != startCat {
		t.Errorf("after h: category = %q, want %q", r.Comments[0].Category, startCat)
	}
}

func TestEditLineIncrementDecrement(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoLineField(e)

	startLine := r.Comments[0].Line
	_, cmd := e.Update(keyRune('l'))
	if cmd == nil {
		t.Error("expected DirtyMsg cmd from line +1")
	}
	if r.Comments[0].Line != startLine+1 {
		t.Errorf("line = %d, want %d", r.Comments[0].Line, startLine+1)
	}

	_, cmd = e.Update(keyRune('h'))
	if cmd == nil {
		t.Error("expected DirtyMsg cmd from line -1")
	}
	if r.Comments[0].Line != startLine {
		t.Errorf("line = %d, want %d", r.Comments[0].Line, startLine)
	}

	r.Comments[0].Line = 1
	_, cmd = e.Update(keyRune('h'))
	if cmd != nil {
		t.Error("no-op cycle should not emit DirtyMsg")
	}
	if r.Comments[0].Line != 1 {
		t.Errorf("clamp at 1, got %d", r.Comments[0].Line)
	}
}

func TestEditLineTextInput(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoLineField(e)

	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !e.LineInputFocused() {
		t.Fatal("Enter on line field did not focus the input")
	}

	e.lineInput.SetValue("")
	e.Update(keyRune('4'))
	e.Update(keyRune('2'))

	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("expected DirtyMsg cmd from line commit")
	}
	if e.LineInputFocused() {
		t.Error("Enter did not blur input after commit")
	}
	if r.Comments[0].Line != 42 {
		t.Errorf("line = %d, want 42", r.Comments[0].Line)
	}
}

func TestEditLineRejectsNonDigit(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})

	e.lineInput.SetValue("")
	e.Update(keyRune('a'))
	if e.lineInput.Value() != "" {
		t.Errorf("non-digit reached buffer: %q", e.lineInput.Value())
	}
}

func TestEditLineRevertsOnInvalidCommit(t *testing.T) {
	t.Parallel()
	r := editFixture()
	original := r.Comments[0].Line
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})

	e.lineInput.SetValue("") // empty -> Atoi fails on the line field
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("invalid commit should not emit DirtyMsg")
	}
	if r.Comments[0].Line != original {
		t.Errorf("line mutated to %d on invalid input; want %d", r.Comments[0].Line, original)
	}
	if e.errMsg == "" {
		t.Error("expected errMsg to be set")
	}
}

func TestEditEscReturnsToDetail(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 1)
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("Esc emitted no cmd")
	}
	msg := cmd()
	got, ok := msg.(GoToDetailMsg)
	if !ok {
		t.Fatalf("Esc cmd produced %T, want GoToDetailMsg", msg)
	}
	if got.Index != 1 {
		t.Errorf("GoToDetailMsg.Index = %d, want 1", got.Index)
	}
}

func TestEditSetIndexResetsState(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !e.LineInputFocused() {
		t.Fatal("setup: input should be focused")
	}

	e.SetIndex(1)
	if e.Index() != 1 {
		t.Errorf("Index = %d, want 1", e.Index())
	}
	if e.Field() != fieldSeverity {
		t.Errorf("field not reset: %v", e.Field())
	}
	if e.LineInputFocused() {
		t.Error("input still focused after SetIndex")
	}
}

// --- start_line ----------------------------------------------------------

func TestEditStartLineCycleFromNone(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)

	if r.Comments[0].StartLine != nil {
		t.Fatalf("setup: StartLine should be nil")
	}
	// h on (none) is a no-op.
	_, cmd := e.Update(keyRune('h'))
	if cmd != nil {
		t.Error("h on (none) start_line should be a no-op")
	}
	if r.Comments[0].StartLine != nil {
		t.Errorf("StartLine became %v", *r.Comments[0].StartLine)
	}

	// l initialises start_line at line-1 = 9.
	_, cmd = e.Update(keyRune('l'))
	if cmd == nil {
		t.Error("l on (none) should emit DirtyMsg when line>1")
	}
	if r.Comments[0].StartLine == nil || *r.Comments[0].StartLine != 9 {
		t.Errorf("after l: StartLine = %v, want 9", r.Comments[0].StartLine)
	}
}

func TestEditStartLineCycleDownToNone(t *testing.T) {
	t.Parallel()
	r := editFixture()
	r.Comments[0].StartLine = ptrInt(1)
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)

	// h from start_line=1 drops back to (none).
	_, cmd := e.Update(keyRune('h'))
	if cmd == nil {
		t.Error("h from 1 should drop range and emit DirtyMsg")
	}
	if r.Comments[0].StartLine != nil {
		t.Errorf("StartLine should be nil, got %v", *r.Comments[0].StartLine)
	}
}

func TestEditStartLineCycleRefusesAtBound(t *testing.T) {
	t.Parallel()
	r := editFixture()
	// line=10, start_line=9 — bumping to 10 would equal line, which we forbid.
	r.Comments[0].StartLine = ptrInt(9)
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)

	_, cmd := e.Update(keyRune('l'))
	if cmd != nil {
		t.Error("l that would push start_line >= line must not emit DirtyMsg")
	}
	if got := *r.Comments[0].StartLine; got != 9 {
		t.Errorf("StartLine = %d, want 9 (unchanged)", got)
	}
	if e.errMsg == "" {
		t.Error("expected errMsg explaining the bound")
	}
}

func TestEditStartLineDropKey(t *testing.T) {
	t.Parallel()
	r := editFixture()
	r.Comments[0].StartLine = ptrInt(5)
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)

	_, cmd := e.Update(keyRune('d'))
	if cmd == nil {
		t.Error("d should emit DirtyMsg when StartLine was set")
	}
	if r.Comments[0].StartLine != nil {
		t.Errorf("StartLine should be nil after d, got %v", *r.Comments[0].StartLine)
	}
}

func TestEditStartLineTextInput(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)

	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !e.StartLineInputFocused() {
		t.Fatal("Enter on start_line did not focus its input")
	}
	e.startLineInput.SetValue("")
	e.Update(keyRune('3'))
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("expected DirtyMsg from start_line commit")
	}
	if r.Comments[0].StartLine == nil || *r.Comments[0].StartLine != 3 {
		t.Errorf("StartLine = %v, want 3", r.Comments[0].StartLine)
	}

	// Empty commit drops the range.
	gotoStartLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	e.startLineInput.SetValue("")
	_, cmd = e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Error("empty commit should drop range and emit DirtyMsg")
	}
	if r.Comments[0].StartLine != nil {
		t.Errorf("StartLine should be nil after empty commit")
	}
}

func TestEditStartLineRejectsAtOrAboveLine(t *testing.T) {
	t.Parallel()
	r := editFixture()
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})

	e.startLineInput.SetValue("")
	// line is 10; entering 10 must be rejected.
	for _, c := range "10" {
		e.Update(keyRune(c))
	}
	_, cmd := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("rejecting commit must not emit DirtyMsg")
	}
	if r.Comments[0].StartLine != nil {
		t.Errorf("StartLine should still be nil; got %v", *r.Comments[0].StartLine)
	}
	if e.errMsg == "" {
		t.Error("expected errMsg explaining the constraint")
	}
}

func TestEditLineCollapseDropsRangeWhenLineLandsAtStart(t *testing.T) {
	t.Parallel()
	r := editFixture()
	r.Comments[0].StartLine = ptrInt(9)
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoLineField(e)

	// h drops line 10 -> 9, which equals start_line; range collapses.
	_, cmd := e.Update(keyRune('h'))
	if cmd == nil {
		t.Error("expected DirtyMsg from line decrement")
	}
	if r.Comments[0].Line != 9 {
		t.Errorf("Line = %d, want 9", r.Comments[0].Line)
	}
	if r.Comments[0].StartLine != nil {
		t.Errorf("StartLine should have collapsed; got %v", *r.Comments[0].StartLine)
	}
}

func TestEditStartLinePreservesStartSide(t *testing.T) {
	t.Parallel()
	r := editFixture()
	left := model.SideLeft
	r.Comments[0].StartLine = ptrInt(5)
	r.Comments[0].StartSide = &left
	e := NewEdit(r, keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)

	// Bumping start_line up keeps StartSide intact.
	e.Update(keyRune('l'))
	if r.Comments[0].StartSide == nil || *r.Comments[0].StartSide != model.SideLeft {
		t.Errorf("StartSide should remain LEFT, got %v", r.Comments[0].StartSide)
	}

	// Dropping the range clears StartSide too — they're meaningless together.
	e.Update(keyRune('d'))
	if r.Comments[0].StartSide != nil {
		t.Errorf("StartSide should be nil after dropping range")
	}
}

func ptrInt(v int) *int { return &v }
