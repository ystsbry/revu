package picker

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ystsbry/revu/internal/github"
)

// findZone renders until id has a known position.
//
// The picker's View() calls Scan itself, so the caller must not scan again
// — a second pass would find no markers. The manager also processes Scan on
// a worker goroutine, so the first render may not have registered positions
// yet; hence the retry.
func findZone(t *testing.T, z *zone.Manager, view func() string, id string) *zone.ZoneInfo {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		view()
		info := z.Get(id)
		if info != nil && !info.IsZero() {
			return info
		}
		if time.Now().After(deadline) {
			t.Fatalf("zone %s never appeared", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func clickAt(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

func wheel(b tea.MouseButton) tea.MouseMsg {
	return tea.MouseMsg{Button: b, Action: tea.MouseActionPress}
}

func key(s string) tea.KeyMsg {
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	switch s {
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	panic("unhandled key: " + s)
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func samplePRs() []github.PRListItem {
	items := make([]github.PRListItem, 3)
	for i := range items {
		items[i].Number = 100 + i
		items[i].Title = []string{"add dashboard", "fix submit", "bump deps"}[i]
		items[i].HeadRefName = []string{"feat/dash", "fix/submit", "chore/deps"}[i]
		items[i].BaseRefName = "main"
		items[i].Author.Login = []string{"alice", "bob", "carol"}[i]
	}
	return items
}

// sizedModel builds a model that has been through a WindowSizeMsg and
// closes its zone manager when the test ends.
func sizedModel(t *testing.T) model {
	t.Helper()
	m := newModel(samplePRs())
	t.Cleanup(m.zones.Close)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(model)
}

func step(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	out, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, want picker.model", next)
	}
	return out, cmd
}

func TestInitReturnsNoCommand(t *testing.T) {
	t.Parallel()
	m := sizedModel(t)
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("Init() = %T, want nil", cmd())
	}
}

func TestWindowSizeMsgRecordsWidth(t *testing.T) {
	t.Parallel()
	m := newModel(samplePRs())
	t.Cleanup(m.zones.Close)

	out, cmd := step(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if out.width != 120 {
		t.Fatalf("width = %d, want 120", out.width)
	}
	if cmd != nil {
		t.Fatalf("WindowSizeMsg should not produce a cmd, got %T", cmd())
	}
}

func TestCursorMovesWithinBounds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		keys []string
		want int
	}{
		{name: "down moves forward", keys: []string{"down"}, want: 1},
		{name: "j moves forward", keys: []string{"j"}, want: 1},
		{name: "up at top stays", keys: []string{"up"}, want: 0},
		{name: "k at top stays", keys: []string{"k"}, want: 0},
		{name: "down stops at the last row", keys: []string{"down", "down", "down", "down"}, want: 2},
		{name: "up after down", keys: []string{"down", "down", "up"}, want: 1},
		{name: "G jumps to the last row", keys: []string{"G"}, want: 2},
		{name: "g jumps back to the first", keys: []string{"G", "g"}, want: 0},
		{name: "unbound key is ignored", keys: []string{"down", "x"}, want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := sizedModel(t)
			for _, k := range tt.keys {
				m, _ = step(t, m, key(k))
			}
			if m.cursor != tt.want {
				t.Fatalf("cursor = %d, want %d", m.cursor, tt.want)
			}
			if m.chose {
				t.Fatal("navigation keys must not select")
			}
		})
	}
}

func TestEnterSelectsCurrentRow(t *testing.T) {
	t.Parallel()
	m := sizedModel(t)
	m, _ = step(t, m, key("down"))

	out, cmd := step(t, m, key("enter"))
	if !out.chose {
		t.Fatal("enter should mark the model as chosen")
	}
	if out.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", out.cursor)
	}
	if !isQuit(cmd) {
		t.Fatal("enter should quit the program")
	}
}

// Quitting must leave chose false — Pick relies on that to return nil
// rather than whatever row the cursor happened to be on.
func TestQuitKeysDoNotSelect(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			m := sizedModel(t)
			m, _ = step(t, m, key("down"))

			out, cmd := step(t, m, key(k))
			if out.chose {
				t.Fatalf("%q must not select", k)
			}
			if !isQuit(cmd) {
				t.Fatalf("%q should quit", k)
			}
		})
	}
}

func TestWheelMovesCursorWithinBounds(t *testing.T) {
	t.Parallel()
	m := sizedModel(t)

	m, _ = step(t, m, wheel(tea.MouseButtonWheelUp))
	if m.cursor != 0 {
		t.Fatalf("wheel up at the top: cursor = %d, want 0", m.cursor)
	}

	for i := 0; i < 5; i++ {
		m, _ = step(t, m, wheel(tea.MouseButtonWheelDown))
	}
	if m.cursor != 2 {
		t.Fatalf("wheel down past the end: cursor = %d, want 2", m.cursor)
	}

	m, _ = step(t, m, wheel(tea.MouseButtonWheelUp))
	if m.cursor != 1 {
		t.Fatalf("wheel up: cursor = %d, want 1", m.cursor)
	}
	if m.chose {
		t.Fatal("the wheel must not select")
	}
}

// The documented contract: a click selects, and clicking the already
// selected row confirms.
func TestClickSelectsThenConfirms(t *testing.T) {
	t.Parallel()
	m := sizedModel(t)
	row := findZone(t, m.zones, m.View, "pick:2")

	m, cmd := step(t, m, clickAt(row.StartX, row.StartY))
	if cmd != nil {
		t.Fatalf("the first click should only move the cursor, got %T", cmd())
	}
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.cursor)
	}
	if m.chose {
		t.Fatal("the first click must not select")
	}

	m, cmd = step(t, m, clickAt(row.StartX, row.StartY))
	if !m.chose {
		t.Fatal("a second click on the same row should select")
	}
	if !isQuit(cmd) {
		t.Fatal("a second click should quit")
	}
}

func TestClickOutsideAnyRowIsIgnored(t *testing.T) {
	t.Parallel()
	m := sizedModel(t)
	findZone(t, m.zones, m.View, "pick:0")

	out, cmd := step(t, m, clickAt(0, 0)) // the title line, above every row
	if cmd != nil {
		t.Fatalf("a click outside the rows should produce no cmd, got %T", cmd())
	}
	if out.cursor != 0 || out.chose {
		t.Fatalf("state changed: cursor=%d chose=%v", out.cursor, out.chose)
	}
}

// Only a left-button press acts. Releases, other buttons and bare motion
// must not move the cursor — otherwise dragging over the list would
// scramble the selection.
func TestNonSelectingMouseEventsAreIgnored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  tea.MouseMsg
	}{
		{
			name: "right button press",
			msg:  tea.MouseMsg{X: 1, Y: 3, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		},
		{
			name: "left button release",
			msg:  tea.MouseMsg{X: 1, Y: 3, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
		},
		{
			name: "motion without a button",
			msg:  tea.MouseMsg{X: 1, Y: 3, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := sizedModel(t)
			findZone(t, m.zones, m.View, "pick:0")

			out, cmd := step(t, m, tt.msg)
			if cmd != nil {
				t.Fatalf("cmd = %T, want nil", cmd())
			}
			if out.cursor != 0 || out.chose {
				t.Fatalf("state changed: cursor=%d chose=%v", out.cursor, out.chose)
			}
		})
	}
}

func TestViewShowsEveryPR(t *testing.T) {
	t.Parallel()
	out := sizedModel(t).View()

	for _, want := range []string{
		"Select a PR to review (3 awaiting)",
		"#100", "#101", "#102",
		"add dashboard", "fix submit", "bump deps",
		"feat/dash", "main", "@alice",
		"click again/enter: select",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() is missing %q\n%s", want, out)
		}
	}
}

func TestViewMarksOnlyTheCursorRow(t *testing.T) {
	t.Parallel()
	m := sizedModel(t)
	m, _ = step(t, m, key("down"))

	var marked []string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "▸") {
			marked = append(marked, l)
		}
	}
	if len(marked) != 1 {
		t.Fatalf("expected exactly one cursor row, got %d: %v", len(marked), marked)
	}
	if !strings.Contains(marked[0], "#101") {
		t.Fatalf("cursor is on %q, want the #101 row", marked[0])
	}
}

func TestPickRejectsEmptyInput(t *testing.T) {
	t.Parallel()
	got, err := Pick(nil)
	if err == nil {
		t.Fatal("Pick(nil) should fail rather than open an empty list")
	}
	if got != nil {
		t.Fatalf("Pick(nil) = %v, want nil", got)
	}
	if !strings.Contains(err.Error(), "no PRs") {
		t.Fatalf("error = %q, want it to mention there are no PRs", err)
	}
}
