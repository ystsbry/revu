package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func findZone(t *testing.T, a *App, id string) (x, y int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		a.View() // runs zones.Scan
		info := a.zones.Get(id)
		if info != nil && !info.IsZero() {
			return info.StartX, info.StartY
		}
		if time.Now().After(deadline) {
			t.Fatalf("zone %s never appeared", id)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func click(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress}
}

// The tab bar switches views on click — the ticket's "ビュー間の切り替えが
// クリックで行える".
func TestTabBarClickSwitchesViews(t *testing.T) {
	t.Parallel()
	a := NewApp(Config{Review: sampleReview()})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	x, y := findZone(t, a, zoneTabSummary)
	a.Update(click(x, y))
	if !a.IsSummary() {
		t.Fatalf("click Summary tab: state should be summary")
	}

	x, y = findZone(t, a, zoneTabList)
	a.Update(click(x, y))
	if !a.IsList() {
		t.Fatalf("click List tab: state should be list")
	}

	x, y = findZone(t, a, zoneTabDetail)
	a.Update(click(x, y))
	if !a.IsDetail() {
		t.Fatalf("click Detail tab: state should be detail")
	}
}

// A click anywhere dismisses the help overlay; wheel events reach the
// active view (regression guard for the app-level routing).
func TestMouseHelpDismissAndRouting(t *testing.T) {
	t.Parallel()
	a := NewApp(Config{Review: sampleReview()})
	a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !a.showHelp {
		t.Fatal("? should open help")
	}
	a.Update(click(5, 5))
	if a.showHelp {
		t.Fatal("click should dismiss help")
	}

	// Wheel down routes to the list: summary row -> first comment.
	a.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if a.list.Cursor() != 0 {
		t.Fatalf("wheel should move the list cursor, got %d", a.list.Cursor())
	}
}
