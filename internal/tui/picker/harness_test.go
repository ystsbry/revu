package picker

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// picker.go and local.go are near-identical bubbletea models (dupl flags
// them as a 60-line duplicate). Rather than mirroring that duplication in
// the tests, the shared contract is described once here and run against
// both models. Model-specific rendering lives in picker_test.go /
// local_test.go.
//
// When REVU-48 collapses the two models into one, this harness collapses
// into a plain test with a single entry.
type harness struct {
	name string
	// zoneID names the clickable region for row i, e.g. "pick:2".
	zoneID func(i int) string
	// newSized returns a model that has already seen a WindowSizeMsg, with
	// its zone manager registered for cleanup.
	newSized func(t *testing.T) tea.Model
	zones    func(tea.Model) *zone.Manager
	cursor   func(tea.Model) int
	chose    func(tea.Model) bool
	// items is how many rows newSized starts with.
	items int
}

func harnesses() []harness {
	return []harness{
		{
			name:     "picker",
			zoneID:   func(i int) string { return fmt.Sprintf("pick:%d", i) },
			newSized: func(t *testing.T) tea.Model { return sizedModel(t) },
			zones:    func(m tea.Model) *zone.Manager { return m.(model).zones },
			cursor:   func(m tea.Model) int { return m.(model).cursor },
			chose:    func(m tea.Model) bool { return m.(model).chose },
			items:    len(samplePRs()),
		},
		{
			name:     "local",
			zoneID:   func(i int) string { return fmt.Sprintf("local:%d", i) },
			newSized: func(t *testing.T) tea.Model { return sizedLocalModel(t) },
			zones:    func(m tea.Model) *zone.Manager { return m.(localModel).zones },
			cursor:   func(m tea.Model) int { return m.(localModel).cursor },
			chose:    func(m tea.Model) bool { return m.(localModel).chose },
			items:    len(sampleLocalItems()),
		},
	}
}

// findZone renders until id has a known position.
//
// Unlike internal/tui/views, the picker's View() calls Scan itself, so the
// caller must not scan again — a second pass would find no markers. The
// manager also processes Scan on a worker goroutine, so the first render
// may not have registered positions yet; hence the retry.
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

func quits(t *testing.T, cmd tea.Cmd) bool {
	t.Helper()
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestInitReturnsNoCommand(t *testing.T) {
	t.Parallel()
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			if cmd := h.newSized(t).Init(); cmd != nil {
				t.Fatalf("Init() = %T, want nil", cmd())
			}
		})
	}
}

func TestWindowSizeMsgIsAbsorbed(t *testing.T) {
	t.Parallel()
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			m, cmd := h.newSized(t).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			if cmd != nil {
				t.Fatalf("WindowSizeMsg should not produce a cmd, got %T", cmd())
			}
			if h.cursor(m) != 0 || h.chose(m) {
				t.Fatal("a resize must not move or select anything")
			}
		})
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
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					m := h.newSized(t)
					for _, k := range tt.keys {
						m, _ = m.Update(key(k))
					}
					if got := h.cursor(m); got != tt.want {
						t.Fatalf("cursor = %d, want %d", got, tt.want)
					}
					if h.chose(m) {
						t.Fatal("navigation keys must not select")
					}
				})
			}
		})
	}
}

func TestEnterSelectsCurrentRow(t *testing.T) {
	t.Parallel()
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			m := h.newSized(t)
			m, _ = m.Update(key("down"))

			m, cmd := m.Update(key("enter"))
			if !h.chose(m) {
				t.Fatal("enter should mark the model as chosen")
			}
			if got := h.cursor(m); got != 1 {
				t.Fatalf("cursor = %d, want 1", got)
			}
			if !quits(t, cmd) {
				t.Fatal("enter should quit the program")
			}
		})
	}
}

// Quitting must leave chose false — Pick / PickLocal rely on that to
// return nil rather than whatever row the cursor happened to be on.
func TestQuitKeysDoNotSelect(t *testing.T) {
	t.Parallel()
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			for _, k := range []string{"q", "esc", "ctrl+c"} {
				t.Run(k, func(t *testing.T) {
					t.Parallel()
					m := h.newSized(t)
					m, _ = m.Update(key("down"))

					m, cmd := m.Update(key(k))
					if h.chose(m) {
						t.Fatalf("%q must not select", k)
					}
					if !quits(t, cmd) {
						t.Fatalf("%q should quit", k)
					}
				})
			}
		})
	}
}

func TestWheelMovesCursorWithinBounds(t *testing.T) {
	t.Parallel()
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			m := h.newSized(t)

			m, _ = m.Update(wheel(tea.MouseButtonWheelUp))
			if got := h.cursor(m); got != 0 {
				t.Fatalf("wheel up at the top: cursor = %d, want 0", got)
			}

			for i := 0; i < h.items+2; i++ {
				m, _ = m.Update(wheel(tea.MouseButtonWheelDown))
			}
			if got, want := h.cursor(m), h.items-1; got != want {
				t.Fatalf("wheel down past the end: cursor = %d, want %d", got, want)
			}

			m, _ = m.Update(wheel(tea.MouseButtonWheelUp))
			if got, want := h.cursor(m), h.items-2; got != want {
				t.Fatalf("wheel up: cursor = %d, want %d", got, want)
			}
			if h.chose(m) {
				t.Fatal("the wheel must not select")
			}
		})
	}
}

// The documented contract: a click selects, and clicking the already
// selected row confirms.
func TestClickSelectsThenConfirms(t *testing.T) {
	t.Parallel()
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			m := h.newSized(t)
			last := h.items - 1
			row := findZone(t, h.zones(m), m.View, h.zoneID(last))

			m, cmd := m.Update(clickAt(row.StartX, row.StartY))
			if cmd != nil {
				t.Fatalf("the first click should only move the cursor, got %T", cmd())
			}
			if got := h.cursor(m); got != last {
				t.Fatalf("cursor = %d, want %d", got, last)
			}
			if h.chose(m) {
				t.Fatal("the first click must not select")
			}

			m, cmd = m.Update(clickAt(row.StartX, row.StartY))
			if !h.chose(m) {
				t.Fatal("a second click on the same row should select")
			}
			if !quits(t, cmd) {
				t.Fatal("a second click should quit")
			}
		})
	}
}

func TestClickOutsideAnyRowIsIgnored(t *testing.T) {
	t.Parallel()
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			m := h.newSized(t)
			findZone(t, h.zones(m), m.View, h.zoneID(0))

			m, cmd := m.Update(clickAt(0, 0)) // the title line, above every row
			if cmd != nil {
				t.Fatalf("a click outside the rows should produce no cmd, got %T", cmd())
			}
			if h.cursor(m) != 0 || h.chose(m) {
				t.Fatalf("state changed: cursor=%d chose=%v", h.cursor(m), h.chose(m))
			}
		})
	}
}

// Only a left-button press acts. Releases, other buttons and bare motion
// must not move the cursor — otherwise dragging over the list would
// scramble the selection.
func TestNonSelectingMouseEventsAreIgnored(t *testing.T) {
	t.Parallel()
	events := []struct {
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
	for _, h := range harnesses() {
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			for _, ev := range events {
				t.Run(ev.name, func(t *testing.T) {
					t.Parallel()
					m := h.newSized(t)
					findZone(t, h.zones(m), m.View, h.zoneID(0))

					m, cmd := m.Update(ev.msg)
					if cmd != nil {
						t.Fatalf("cmd = %T, want nil", cmd())
					}
					if h.cursor(m) != 0 || h.chose(m) {
						t.Fatalf("state changed: cursor=%d chose=%v", h.cursor(m), h.chose(m))
					}
				})
			}
		})
	}
}
