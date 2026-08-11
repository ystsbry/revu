package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ystsbry/revu/internal/filter"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

func TestPadCell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{name: "pads to the column width", s: "ab", width: 5, want: "ab   "},
		{name: "exact width is untouched", s: "abcde", width: 5, want: "abcde"},
		{name: "over width gets an ellipsis", s: "abcdef", width: 5, want: "abcd…"},
		// A one-cell column has no room for both a character and an
		// ellipsis, so it is cut instead.
		{name: "single cell column", s: "abcdef", width: 1, want: "a"},
		{name: "zero width column", s: "abc", width: 0, want: ""},
		{name: "empty string pads", s: "", width: 3, want: "   "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := padCell(tt.s, tt.width); got != tt.want {
				t.Fatalf("padCell(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{in: "abc1234567890", want: "abc1234"},
		{in: "abc1234", want: "abc1234"},
		{in: "abc", want: "abc"},
		{in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()
			if got := shortSHA(tt.in); got != tt.want {
				t.Fatalf("shortSHA(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// FilterExpr is what the list header echoes back, so it has to round-trip
// the expression the user typed.
func TestListFilterExpr(t *testing.T) {
	t.Parallel()
	l := NewList(editFixture(), keys.DefaultKeyMap())

	if got := l.FilterExpr(); got != "" {
		t.Fatalf("FilterExpr() = %q, want empty before any filter", got)
	}

	f, err := filter.Parse("severity:major")
	if err != nil {
		t.Fatal(err)
	}
	l.SetFilter(f)
	if got := l.FilterExpr(); !strings.Contains(got, "major") {
		t.Fatalf("FilterExpr() = %q, want the applied expression", got)
	}

	l.ClearFilter()
	if got := l.FilterExpr(); got != "" {
		t.Fatalf("FilterExpr() after clear = %q, want empty", got)
	}
}

// SetIndex is how the app re-enters the detail view at a given comment; it
// clamps rather than trusting the caller.
func TestDetailSetIndexClamps(t *testing.T) {
	t.Parallel()
	r := editFixture() // two comments
	d := NewDetail(r, t.TempDir(), keys.DefaultKeyMap(), 0, DetailSettings{})

	d.SetIndex(1)
	if d.Index() != 1 {
		t.Fatalf("Index() = %d, want 1", d.Index())
	}

	d.SetIndex(99)
	if d.Index() != 1 {
		t.Fatalf("Index() = %d, want it clamped to the last comment", d.Index())
	}

	d.SetIndex(-5)
	if d.Index() != 0 {
		t.Fatalf("Index() = %d, want it clamped to the first comment", d.Index())
	}
}

// Moving to another comment resets the markdown scroll: keeping the old
// offset would open the next comment part-way down its body.
func TestDetailSetIndexResetsScroll(t *testing.T) {
	t.Parallel()
	d := NewDetail(editFixture(), t.TempDir(), keys.DefaultKeyMap(), 0, DetailSettings{})
	d.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	d.mdScroll = 5

	d.SetIndex(1)
	if d.mdScroll != 0 {
		t.Fatalf("mdScroll = %d, want it reset on a new comment", d.mdScroll)
	}
}

// The markdown pane height drives half-page scrolling, so it must stay
// positive even on a terminal too small to hold the chrome.
func TestDetailMarkdownContentHeight(t *testing.T) {
	t.Parallel()
	d := NewDetail(editFixture(), t.TempDir(), keys.DefaultKeyMap(), 0, DetailSettings{})

	d.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	if got := d.markdownContentHeight(); got != 26 {
		t.Fatalf("markdownContentHeight at height 30 = %d, want 26", got)
	}

	d.Update(tea.WindowSizeMsg{Width: 100, Height: 1})
	if got := d.markdownContentHeight(); got < 1 {
		t.Fatalf("markdownContentHeight on a tiny terminal = %d, want at least 1", got)
	}
}

func TestSummaryScrollByClampsAtBothEnds(t *testing.T) {
	t.Parallel()
	s := NewSummary(editFixture(), keys.DefaultKeyMap())
	s.maxScroll = 10

	s.scrollBy(-3)
	if s.scroll != 0 {
		t.Fatalf("scrolling up from the top = %d, want 0", s.scroll)
	}

	s.scrollBy(4)
	if s.scroll != 4 {
		t.Fatalf("scroll = %d, want 4", s.scroll)
	}

	s.scrollBy(100)
	if s.scroll != 10 {
		t.Fatalf("scrolling past the end = %d, want the max of 10", s.scroll)
	}
}

func TestSummaryWheelScrolls(t *testing.T) {
	t.Parallel()
	s := NewSummary(editFixture(), keys.DefaultKeyMap())
	s.maxScroll = 10

	s.Update(tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if s.scroll != 3 {
		t.Fatalf("wheel down = %d, want 3", s.scroll)
	}
	s.Update(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if s.scroll != 0 {
		t.Fatalf("wheel up = %d, want 0", s.scroll)
	}
}

// Anything that is not a wheel event or a left-button press leaves the
// summary alone.
func TestSummaryIgnoresNonSelectingMouseEvents(t *testing.T) {
	t.Parallel()
	for _, msg := range []tea.MouseMsg{
		{X: 1, Y: 1, Button: tea.MouseButtonRight, Action: tea.MouseActionPress},
		{X: 1, Y: 1, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
	} {
		s := NewSummary(editFixture(), keys.DefaultKeyMap())
		before := s.review.ReviewEvent

		if _, cmd := s.Update(msg); cmd != nil {
			t.Fatalf("%v produced %T, want nothing", msg, cmd())
		}
		if s.review.ReviewEvent != before {
			t.Fatalf("%v changed the review event", msg)
		}
	}
}

// Clicking the already-selected event is a no-op: re-emitting dirty would
// mark an unchanged review as needing a save.
func TestSummaryClickingTheCurrentEventIsANoOp(t *testing.T) {
	t.Parallel()
	r := editFixture()
	r.ReviewEvent = model.EventComment
	s := NewSummary(r, keys.DefaultKeyMap())
	z := zone.New()
	t.Cleanup(z.Close)
	s.AttachZones(z)
	s.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	info := scanAndFind(t, z, s.View, ZoneSummaryEventPrefix+string(model.EventComment))
	_, cmd := s.Update(tea.MouseMsg{
		X: info.StartX, Y: info.StartY,
		Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	if cmd != nil {
		t.Fatalf("re-selecting the current event produced %T, want nothing", cmd())
	}
	if r.ReviewEvent != model.EventComment {
		t.Fatalf("event = %v, want it unchanged", r.ReviewEvent)
	}
}
