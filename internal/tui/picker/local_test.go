package picker

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleLocalItems() []LocalPRItem {
	now := time.Now()
	return []LocalPRItem{
		{Number: 12, ShortSHA: "abc1234", Path: "/tmp/revu/pr-12/abc1234", GeneratedAt: now.Add(-90 * time.Minute)},
		{Number: 34, ShortSHA: "def5678", Path: "/tmp/revu/pr-34/def5678", GeneratedAt: now.Add(-3 * time.Minute)},
		{Number: 56, ShortSHA: "", Path: "/tmp/revu/pr-56/0000000"},
	}
}

func sizedLocalModel(t *testing.T) localModel {
	t.Helper()
	m := newLocalModel(sampleLocalItems())
	t.Cleanup(m.zones.Close)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return next.(localModel)
}

func stepLocal(t *testing.T, m localModel, msg tea.Msg) (localModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	out, ok := next.(localModel)
	if !ok {
		t.Fatalf("Update returned %T, want picker.localModel", next)
	}
	return out, cmd
}

func TestLocalInitReturnsNoCommand(t *testing.T) {
	t.Parallel()
	m := sizedLocalModel(t)
	if cmd := m.Init(); cmd != nil {
		t.Fatalf("Init() = %T, want nil", cmd())
	}
}

func TestLocalWindowSizeMsgRecordsWidth(t *testing.T) {
	t.Parallel()
	m := newLocalModel(sampleLocalItems())
	t.Cleanup(m.zones.Close)

	out, cmd := stepLocal(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	if out.width != 80 {
		t.Fatalf("width = %d, want 80", out.width)
	}
	if cmd != nil {
		t.Fatalf("WindowSizeMsg should not produce a cmd, got %T", cmd())
	}
}

func TestLocalCursorMovesWithinBounds(t *testing.T) {
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
			m := sizedLocalModel(t)
			for _, k := range tt.keys {
				m, _ = stepLocal(t, m, key(k))
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

func TestLocalEnterSelectsCurrentRow(t *testing.T) {
	t.Parallel()
	m := sizedLocalModel(t)
	m, _ = stepLocal(t, m, key("down"))

	out, cmd := stepLocal(t, m, key("enter"))
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

// Quitting must leave chose false — PickLocal relies on that to return nil
// rather than whatever row the cursor happened to be on.
func TestLocalQuitKeysDoNotSelect(t *testing.T) {
	t.Parallel()
	for _, k := range []string{"q", "esc", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			t.Parallel()
			m := sizedLocalModel(t)
			m, _ = stepLocal(t, m, key("down"))

			out, cmd := stepLocal(t, m, key(k))
			if out.chose {
				t.Fatalf("%q must not select", k)
			}
			if !isQuit(cmd) {
				t.Fatalf("%q should quit", k)
			}
		})
	}
}

func TestLocalWheelMovesCursorWithinBounds(t *testing.T) {
	t.Parallel()
	m := sizedLocalModel(t)

	m, _ = stepLocal(t, m, wheel(tea.MouseButtonWheelUp))
	if m.cursor != 0 {
		t.Fatalf("wheel up at the top: cursor = %d, want 0", m.cursor)
	}

	for i := 0; i < 5; i++ {
		m, _ = stepLocal(t, m, wheel(tea.MouseButtonWheelDown))
	}
	if m.cursor != 2 {
		t.Fatalf("wheel down past the end: cursor = %d, want 2", m.cursor)
	}

	m, _ = stepLocal(t, m, wheel(tea.MouseButtonWheelUp))
	if m.cursor != 1 {
		t.Fatalf("wheel up: cursor = %d, want 1", m.cursor)
	}
	if m.chose {
		t.Fatal("the wheel must not select")
	}
}

func TestLocalClickSelectsThenConfirms(t *testing.T) {
	t.Parallel()
	m := sizedLocalModel(t)
	row := findZone(t, m.zones, m.View, "local:2")

	m, cmd := stepLocal(t, m, clickAt(row.StartX, row.StartY))
	if cmd != nil {
		t.Fatalf("the first click should only move the cursor, got %T", cmd())
	}
	if m.cursor != 2 {
		t.Fatalf("cursor = %d, want 2", m.cursor)
	}
	if m.chose {
		t.Fatal("the first click must not select")
	}

	m, cmd = stepLocal(t, m, clickAt(row.StartX, row.StartY))
	if !m.chose {
		t.Fatal("a second click on the same row should select")
	}
	if !isQuit(cmd) {
		t.Fatal("a second click should quit")
	}
}

func TestLocalClickOutsideAnyRowIsIgnored(t *testing.T) {
	t.Parallel()
	m := sizedLocalModel(t)
	findZone(t, m.zones, m.View, "local:0")

	out, cmd := stepLocal(t, m, clickAt(0, 0)) // the title line, above every row
	if cmd != nil {
		t.Fatalf("a click outside the rows should produce no cmd, got %T", cmd())
	}
	if out.cursor != 0 || out.chose {
		t.Fatalf("state changed: cursor=%d chose=%v", out.cursor, out.chose)
	}
}

func TestLocalNonSelectingMouseEventsAreIgnored(t *testing.T) {
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
			m := sizedLocalModel(t)
			findZone(t, m.zones, m.View, "local:0")

			out, cmd := stepLocal(t, m, tt.msg)
			if cmd != nil {
				t.Fatalf("cmd = %T, want nil", cmd())
			}
			if out.cursor != 0 || out.chose {
				t.Fatalf("state changed: cursor=%d chose=%v", out.cursor, out.chose)
			}
		})
	}
}

func TestLocalViewShowsShaAndAge(t *testing.T) {
	t.Parallel()
	out := sizedLocalModel(t).View()

	for _, want := range []string{
		"Open a reviewed PR (3)",
		"#12", "#34", "#56",
		"abc1234", "def5678",
		"reviewed 1h ago", // 90 minutes → hours bucket
		"reviewed 3m ago", // 3 minutes → minutes bucket
		"(unknown time)",  // zero GeneratedAt
		"click again/enter: select",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("View() is missing %q\n%s", want, out)
		}
	}
}

// A review directory can exist without a recorded SHA; the row must still
// render rather than leaving an empty gap.
func TestLocalViewFallsBackWhenShaIsMissing(t *testing.T) {
	t.Parallel()
	m := newLocalModel([]LocalPRItem{{Number: 7}})
	t.Cleanup(m.zones.Close)

	if out := m.View(); !strings.Contains(out, "- · reviewed") {
		t.Fatalf("a missing SHA should render as %q\n%s", "-", out)
	}
}

func TestLocalViewMarksOnlyTheCursorRow(t *testing.T) {
	t.Parallel()
	m := sizedLocalModel(t)
	m, _ = stepLocal(t, m, key("G"))

	var marked []string
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "▸") {
			marked = append(marked, l)
		}
	}
	if len(marked) != 1 {
		t.Fatalf("expected exactly one cursor row, got %d: %v", len(marked), marked)
	}
	if !strings.Contains(marked[0], "#56") {
		t.Fatalf("cursor is on %q, want the last row (#56)", marked[0])
	}
}

func TestPickLocalRejectsEmptyInput(t *testing.T) {
	t.Parallel()
	got, err := PickLocal(nil)
	if err == nil {
		t.Fatal("PickLocal(nil) should fail rather than open an empty list")
	}
	if got != nil {
		t.Fatalf("PickLocal(nil) = %v, want nil", got)
	}
	if !strings.Contains(err.Error(), "no reviewed PRs") {
		t.Fatalf("error = %q, want it to mention there are no reviewed PRs", err)
	}
}

func TestFormatRelTime(t *testing.T) {
	t.Parallel()
	now := time.Now()
	old := time.Date(2020, 3, 4, 5, 6, 7, 0, time.UTC)
	future := now.Add(time.Hour)

	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{name: "zero value", in: time.Time{}, want: "(unknown time)"},
		{name: "future falls back to absolute", in: future, want: future.Format(time.RFC3339)},
		{name: "under a minute", in: now.Add(-30 * time.Second), want: "just now"},
		{name: "just over a minute", in: now.Add(-61 * time.Second), want: "1m ago"},
		{name: "minutes", in: now.Add(-5 * time.Minute), want: "5m ago"},
		{name: "just under an hour", in: now.Add(-59 * time.Minute), want: "59m ago"},
		{name: "hours", in: now.Add(-5 * time.Hour), want: "5h ago"},
		{name: "just under a day", in: now.Add(-23 * time.Hour), want: "23h ago"},
		{name: "days", in: now.Add(-3 * 24 * time.Hour), want: "3d ago"},
		{name: "beyond 30 days is a date", in: old, want: "2020-03-04"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatRelTime(tt.in); got != tt.want {
				t.Fatalf("formatRelTime(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
