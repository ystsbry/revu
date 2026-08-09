package views

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// scanAndFind renders via view() through the zone manager until id is
// known. The manager processes Scan on a worker goroutine, so the first
// render may not have registered positions yet.
func scanAndFind(t *testing.T, z *zone.Manager, view func() string, id string) *zone.ZoneInfo {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		z.Scan(view())
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

func sizedList(t *testing.T, r *model.Review) (*List, *zone.Manager) {
	t.Helper()
	z := zone.New()
	t.Cleanup(z.Close)
	l := NewList(r, keys.DefaultKeyMap())
	l.AttachZones(z)
	l.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	return l, z
}

// Click selects; a second click on the same row opens the detail view —
// the ticket's "選択済み行の再クリックで詳細へ".
func TestListClickSelectsThenOpens(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l, z := sizedList(t, r)

	row := scanAndFind(t, z, l.View, ZoneListRowPrefix+"1") // second visible row = c2
	_, cmd := l.Update(clickAt(row.StartX, row.StartY))
	if cmd != nil {
		t.Fatalf("first click should only select, got cmd %T", cmd())
	}
	if l.Cursor() != 1 {
		t.Fatalf("cursor = %d, want 1 (c2)", l.Cursor())
	}

	_, cmd = l.Update(clickAt(row.StartX, row.StartY))
	if cmd == nil {
		t.Fatal("second click should open the row")
	}
	msg, ok := cmd().(GoToDetailMsg)
	if !ok || msg.Index != 1 {
		t.Fatalf("got %#v, want GoToDetailMsg{Index: 1}", cmd())
	}
}

func TestListClickSummaryRow(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l, z := sizedList(t, r)
	pressJ(l) // move off the summary row first

	sum := scanAndFind(t, z, l.View, ZoneListSummaryRow)
	l.Update(clickAt(sum.StartX, sum.StartY))
	if l.Cursor() != summaryCursor {
		t.Fatalf("cursor = %d, want summary row", l.Cursor())
	}
	_, cmd := l.Update(clickAt(sum.StartX, sum.StartY))
	if cmd == nil {
		t.Fatal("second click should open the summary")
	}
	if _, ok := cmd().(GoToSummaryMsg); !ok {
		t.Fatalf("got %T, want GoToSummaryMsg", cmd())
	}
}

func TestListWheelMovesCursor(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	l, _ := sizedList(t, r)

	l.Update(wheel(tea.MouseButtonWheelDown))
	if l.Cursor() != 0 {
		t.Fatalf("wheel down from summary should land on c1, cursor = %d", l.Cursor())
	}
	l.Update(wheel(tea.MouseButtonWheelUp))
	if l.Cursor() != summaryCursor {
		t.Fatalf("wheel up should return to summary, cursor = %d", l.Cursor())
	}
}

func TestDetailActionBarClicks(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	z := zone.New()
	t.Cleanup(z.Close)
	d := NewDetail(r, "", keys.DefaultKeyMap(), 0, DetailSettings{PreImage: nullPreImage{}})
	d.AttachZones(z)
	d.Update(tea.WindowSizeMsg{Width: 120, Height: 40})

	accept := scanAndFind(t, z, d.View, ZoneDetailAccept)
	_, cmd := d.Update(clickAt(accept.StartX, accept.StartY))
	if r.Comments[0].Status != model.StatusAccepted {
		t.Fatalf("click accept: status = %s", r.Comments[0].Status)
	}
	if cmd == nil {
		t.Fatal("accept click should emit DirtyMsg")
	}
	if _, ok := cmd().(DirtyMsg); !ok {
		t.Fatalf("got %T, want DirtyMsg", cmd())
	}

	next := scanAndFind(t, z, d.View, ZoneDetailNext)
	d.Update(clickAt(next.StartX, next.StartY))
	if d.Index() != 1 {
		t.Fatalf("click next: index = %d, want 1", d.Index())
	}

	list := scanAndFind(t, z, d.View, ZoneDetailList)
	_, cmd = d.Update(clickAt(list.StartX, list.StartY))
	if cmd == nil {
		t.Fatal("list button should emit GoToListMsg")
	}
	if _, ok := cmd().(GoToListMsg); !ok {
		t.Fatalf("got %T, want GoToListMsg", cmd())
	}
}

func TestDetailWheelScrollsMarkdown(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	d := NewDetail(r, "", keys.DefaultKeyMap(), 0, DetailSettings{PreImage: nullPreImage{}})
	d.mdMaxScroll = 10

	d.Update(wheel(tea.MouseButtonWheelDown))
	if d.mdScroll != 3 {
		t.Fatalf("wheel down: mdScroll = %d, want 3", d.mdScroll)
	}
	d.Update(wheel(tea.MouseButtonWheelUp))
	if d.mdScroll != 0 {
		t.Fatalf("wheel up: mdScroll = %d, want 0", d.mdScroll)
	}
}

func TestSummaryEventClickSetsDirectly(t *testing.T) {
	t.Parallel()
	r := sampleReview()
	z := zone.New()
	t.Cleanup(z.Close)
	s := NewSummary(r, keys.DefaultKeyMap())
	s.AttachZones(z)
	s.Update(tea.WindowSizeMsg{Width: 100, Height: 30})

	req := scanAndFind(t, z, s.View, ZoneSummaryEventPrefix+string(model.EventRequestChanges))
	_, cmd := s.Update(clickAt(req.StartX, req.StartY))
	if r.ReviewEvent != model.EventRequestChanges {
		t.Fatalf("event = %s, want REQUEST_CHANGES", r.ReviewEvent)
	}
	if cmd == nil {
		t.Fatal("event change should emit DirtyMsg")
	}

	// Clicking the already-active event is a no-op, not a dirty change.
	req = scanAndFind(t, z, s.View, ZoneSummaryEventPrefix+string(model.EventRequestChanges))
	_, cmd = s.Update(clickAt(req.StartX, req.StartY))
	if cmd != nil {
		t.Fatal("re-clicking the active event must not dirty the review")
	}
}

// nullPreImage keeps detail tests off git.
type nullPreImage struct{}

func (nullPreImage) Content(string) ([]byte, error)   { return nil, nil }
func (nullPreImage) PostImage(string) ([]byte, error) { return nil, nil }
func (nullPreImage) Diff(string) ([]byte, error)      { return nil, nil }
