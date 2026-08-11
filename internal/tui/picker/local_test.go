package picker

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Interaction behaviour shared with model lives in harness_test.go; this
// file covers what is specific to the local (already-reviewed) picker.

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

func TestLocalModelRecordsWidth(t *testing.T) {
	t.Parallel()
	m := newLocalModel(sampleLocalItems())
	t.Cleanup(m.zones.Close)

	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := next.(localModel).width; got != 80 {
		t.Fatalf("width = %d, want 80", got)
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
	next, _ := m.Update(key("G"))

	var marked []string
	for _, l := range strings.Split(next.View(), "\n") {
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
		// The minute boundary is the one most likely to break if the
		// thresholds are ever reordered, so both sides are pinned.
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
