package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// The focus marker is what tells the user which row the arrow keys will
// affect, so exactly one row may carry it at a time.
func markedRows(view string) []string {
	var out []string
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "▶") {
			out = append(out, l)
		}
	}
	return out
}

func TestEditViewShowsEveryFieldRow(t *testing.T) {
	t.Parallel()
	e := NewEdit(editFixture(), keys.DefaultKeyMap(), 0)
	out := e.View()

	for _, want := range []string{
		"Edit — c1", "x.go", "pending",
		"severity", "category", "start_line", "line",
		// The current values must be visible, not just the labels.
		"major", "design",
		"[Enter]edit",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() is missing %q:\n%s", want, out)
		}
	}
}

func TestEditViewMarksTheFocusedRowOnly(t *testing.T) {
	t.Parallel()
	e := NewEdit(editFixture(), keys.DefaultKeyMap(), 0)

	for _, want := range []string{"severity", "category", "start_line", "line"} {
		marked := markedRows(e.View())
		if len(marked) != 1 {
			t.Fatalf("focused on %s: %d marked rows, want exactly 1", want, len(marked))
		}
		if !strings.Contains(marked[0], want) {
			t.Fatalf("marker is on %q, want the %s row", marked[0], want)
		}
		e.Update(keyRune('j'))
	}
}

// A single-line comment has no start_line; the row has to say so rather
// than render an empty gap.
func TestEditViewShowsNoneForAnAbsentStartLine(t *testing.T) {
	t.Parallel()
	e := NewEdit(editFixture(), keys.DefaultKeyMap(), 0)

	if out := e.View(); !strings.Contains(out, "(none)") {
		t.Fatalf("View() should mark the empty start_line:\n%s", out)
	}
}

func TestEditViewShowsAnExistingStartLine(t *testing.T) {
	t.Parallel()
	r := editFixture()
	start := 7
	r.Comments[0].StartLine = &start
	e := NewEdit(r, keys.DefaultKeyMap(), 0)

	out := e.View()
	if strings.Contains(out, "(none)") {
		t.Fatalf("a set start_line must not render as (none):\n%s", out)
	}
	if !strings.Contains(out, "7") {
		t.Fatalf("View() should show the start line:\n%s", out)
	}
}

// While a number is being typed the row shows the text input instead of
// the stored value.
func TestEditViewShowsTheInputWhileEditing(t *testing.T) {
	t.Parallel()
	e := NewEdit(editFixture(), keys.DefaultKeyMap(), 0)
	gotoLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !e.lineInput.Focused() {
		t.Fatal("enter on the line row should focus the input")
	}
	e.Update(keyRune('4'))
	if out := e.View(); !strings.Contains(out, "4") {
		t.Fatalf("View() should show what is being typed:\n%s", out)
	}
}

func TestEditViewShowsTheStartLineInputWhileEditing(t *testing.T) {
	t.Parallel()
	e := NewEdit(editFixture(), keys.DefaultKeyMap(), 0)
	gotoStartLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if !e.startLineInput.Focused() {
		t.Fatal("enter on the start_line row should focus the input")
	}
	e.Update(keyRune('3'))
	if out := e.View(); !strings.Contains(out, "3") {
		t.Fatalf("View() should show what is being typed:\n%s", out)
	}
}

// The side is part of the line row's hint because it decides which file
// version the number refers to.
func TestEditViewShowsTheSideOnTheLineRow(t *testing.T) {
	t.Parallel()
	r := editFixture()
	r.Comments[0].Side = model.SideLeft
	e := NewEdit(r, keys.DefaultKeyMap(), 0)

	if out := e.View(); !strings.Contains(out, "side=LEFT") {
		t.Fatalf("View() should name the side:\n%s", out)
	}
}

// A validation failure has to stay on screen; otherwise the user retypes
// the same bad value.
func TestEditViewShowsValidationErrors(t *testing.T) {
	t.Parallel()
	e := NewEdit(editFixture(), keys.DefaultKeyMap(), 0)
	gotoLineField(e)
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// The input starts pre-filled with the current line, so clear it
	// before typing the invalid value.
	for i := 0; i < 4; i++ {
		e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	e.Update(keyRune('0'))
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if e.errMsg == "" {
		t.Fatal("line 0 should be rejected")
	}
	if out := e.View(); !strings.Contains(out, e.errMsg) {
		t.Fatalf("View() should surface %q:\n%s", e.errMsg, out)
	}
}

// A review with no comments still has to render something rather than
// index past the end.
func TestEditViewWithoutComments(t *testing.T) {
	t.Parallel()
	e := NewEdit(&model.Review{PR: model.PRMeta{Repo: "o/r", Number: 1}}, keys.DefaultKeyMap(), 0)

	if out := e.View(); out != "no comments" {
		t.Fatalf("View() = %q, want the empty-state text", out)
	}
}

// Every severity and category the registry knows about must appear as an
// option, so cycling never lands on something the user cannot see.
func TestEditViewListsEveryEnumOption(t *testing.T) {
	t.Parallel()
	e := NewEdit(editFixture(), keys.DefaultKeyMap(), 0)
	out := e.View()

	for _, d := range model.ActiveSeverityRegistry().All() {
		if !strings.Contains(out, d.Name) {
			t.Errorf("severity %q is missing from the row:\n%s", d.Name, out)
		}
	}
	for _, c := range allCategories {
		if !strings.Contains(out, string(c)) {
			t.Errorf("category %q is missing from the row:\n%s", c, out)
		}
	}
}
