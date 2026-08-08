package views

import (
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// Zone ids used across the views. Exported pieces live here so the app
// (which owns the zone manager and does the final Scan) and the views
// (which Mark their clickable regions) agree on names.
const (
	ZoneListSummaryRow = "list:summary"
	ZoneListRowPrefix  = "list:row:" // + visible-row index

	ZoneDetailAccept  = "detail:accept"
	ZoneDetailReject  = "detail:reject"
	ZoneDetailPending = "detail:pending"
	ZoneDetailPrev    = "detail:prev"
	ZoneDetailNext    = "detail:next"
	ZoneDetailEdit    = "detail:edit"
	ZoneDetailList    = "detail:list"

	ZoneSummaryList        = "summary:list"
	ZoneSummaryEdit        = "summary:edit"
	ZoneSummaryEventPrefix = "summary:event:" // + review event name
)

// mark wraps s in a clickable zone when a manager is attached. Views are
// fully functional without one (keyboard-only, plain rendering), which is
// what every pre-mouse test exercises.
func mark(z *zone.Manager, id, s string) string {
	if z == nil {
		return s
	}
	return z.Mark(id, s)
}

// hit reports whether the mouse event lands in the named zone. False when
// no manager is attached or the zone has not been rendered yet.
func hit(z *zone.Manager, id string, msg tea.MouseMsg) bool {
	if z == nil {
		return false
	}
	info := z.Get(id)
	return info != nil && !info.IsZero() && info.InBounds(msg)
}

// leftClick reports a left-button press — the "click" every view acts on.
func leftClick(msg tea.MouseMsg) bool {
	return msg.Button == tea.MouseButtonLeft && msg.Action == tea.MouseActionPress
}
