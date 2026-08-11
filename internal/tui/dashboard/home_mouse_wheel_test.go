package dashboard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func wheelAt(b tea.MouseButton, x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Button: b, Action: tea.MouseActionPress}
}

// The wheel scrolls the job tab when that tab is showing, without the
// caller having to focus it first.
func TestHomeWheelScrollsTheJobTab(t *testing.T) {
	t.Parallel()
	h := homeWithJobs(t, 10, 3+cardHeight*3)

	for i := 0; i < 4; i++ {
		h.Update(wheelAt(tea.MouseButtonWheelDown, 60, 10))
	}
	if h.jobCursor != 4 {
		t.Fatalf("jobCursor = %d, want 4 after four wheel-downs", h.jobCursor)
	}
	if h.jobOffset != 2 {
		t.Fatalf("jobOffset = %d, want 2 (window follows the cursor)", h.jobOffset)
	}
	if h.focus != focusPRList {
		t.Errorf("scrolling the card pane should focus it")
	}

	for i := 0; i < 10; i++ {
		h.Update(wheelAt(tea.MouseButtonWheelUp, 60, 10))
	}
	if h.jobCursor != 0 || h.jobOffset != 0 {
		t.Fatalf("cursor=%d offset=%d, want both back at the top", h.jobCursor, h.jobOffset)
	}
}

func TestHomeWheelScrollsThePRCards(t *testing.T) {
	t.Parallel()
	h := homeWithPRs(t, 10, 3+cardHeight*3)

	for i := 0; i < 4; i++ {
		h.Update(wheelAt(tea.MouseButtonWheelDown, 60, 10))
	}
	if h.prCursor != 4 {
		t.Fatalf("prCursor = %d, want 4", h.prCursor)
	}
	if h.prOffset != 2 {
		t.Fatalf("prOffset = %d, want 2", h.prOffset)
	}

	// The wheel never runs past either end of the list.
	for i := 0; i < 20; i++ {
		h.Update(wheelAt(tea.MouseButtonWheelDown, 60, 10))
	}
	if h.prCursor != 9 {
		t.Fatalf("prCursor = %d, want it clamped to the last card", h.prCursor)
	}
}

// Anything that is not a wheel event or a left-button press is ignored, so
// dragging across the dashboard cannot change the selection.
func TestHomeIgnoresNonSelectingMouseEvents(t *testing.T) {
	t.Parallel()
	for _, msg := range []tea.MouseMsg{
		{X: 60, Y: 10, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		{X: 60, Y: 10, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
		{X: 60, Y: 10, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion},
	} {
		h := homeWithPRs(t, 5, 40)
		h.prCursor = 2

		if _, cmd := h.Update(msg); cmd != nil {
			t.Fatalf("%v produced %T, want nothing", msg, cmd())
		}
		if h.prCursor != 2 {
			t.Fatalf("%v moved the cursor to %d", msg, h.prCursor)
		}
	}
}
