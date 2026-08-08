package views

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ystsbry/revu/internal/filter"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// summaryCursor indicates the summary row is selected. Any non-negative value
// is an index into review.Comments.
const summaryCursor = -1

// listColumns is the fixed table layout: (title, width) pairs.
var listColumns = []struct {
	title string
	width int
}{
	{"#", 4},
	{"Status", 9},
	{"Sev", 8},
	{"Cat", 8},
	{"File:Line", 50},
}

// List renders the comment table with a summary selector row above it,
// optionally filtered by a filter.Filter expression.
//
// Rows are rendered by hand (not bubbles/table) so every row can carry a
// mouse zone: click to select, click the selected row again to open it.
type List struct {
	keys   keys.KeyMap
	review *model.Review
	cursor int
	zones  *zone.Manager

	// offset is the first visible-row index shown in the table viewport;
	// rowsHeight is how many rows fit. Kept explicit so mouse hit-testing
	// can map a clicked row back to a comment.
	offset     int
	rowsHeight int

	// Filtering.
	filter      filter.Filter
	visibleIdx  []int
	filterMode  bool
	filterInput textinput.Model

	width  int
	height int
}

// DirtyMsg is emitted when the in-memory Review has been mutated.
type DirtyMsg struct{}

// GoToDetailMsg requests the parent app to switch to the detail view at Index.
type GoToDetailMsg struct{ Index int }

// GoToSummaryMsg requests the parent app to switch to the summary view.
type GoToSummaryMsg struct{}

// FilterErrMsg is emitted when an entered filter expression fails to parse.
// The parent app can show this in its status bar.
type FilterErrMsg struct{ Err error }

func NewList(r *model.Review, km keys.KeyMap) *List {
	fi := textinput.New()
	fi.Prompt = "/"
	fi.CharLimit = 128
	fi.Placeholder = "severity:major,critical category:bug ..."

	l := &List{keys: km, review: r, cursor: summaryCursor, filterInput: fi, rowsHeight: 10}
	l.recomputeVisible()
	return l
}

// AttachZones enables mouse hit-testing through the app's zone manager.
func (l *List) AttachZones(z *zone.Manager) { l.zones = z }

func (l *List) Init() tea.Cmd { return nil }

// IsFilterMode reports whether the list is currently capturing keys for the
// filter input. The parent app uses this to suppress its own command mode.
func (l *List) IsFilterMode() bool { return l.filterMode }

// SetFilter applies f and resets the cursor to the first visible comment
// (or summary row when nothing is visible).
func (l *List) SetFilter(f filter.Filter) {
	l.filter = f
	l.recomputeVisible()
	if len(l.visibleIdx) > 0 {
		l.cursor = l.visibleIdx[0]
	} else {
		l.cursor = summaryCursor
	}
	l.offset = 0
}

// ClearFilter resets to the no-filter state.
func (l *List) ClearFilter() { l.SetFilter(filter.Filter{}) }

// FilterExpr returns the current filter expression for display purposes.
func (l *List) FilterExpr() string { return l.filter.String() }

func (l *List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height
		// Header (1) + filter line (0-1) + summary row (1) + blank (1) + footer (1) + table header/rule (2).
		extra := 6
		if l.filterMode || !l.filter.IsEmpty() {
			extra++
		}
		h := msg.Height - extra
		if h < 5 {
			h = 5
		}
		l.rowsHeight = h
		l.filterInput.Width = msg.Width - 4
		l.ensureCursorVisible()

	case tea.MouseMsg:
		return l.updateMouse(msg)

	case tea.KeyMsg:
		if l.filterMode {
			return l.updateFilterMode(msg)
		}
		s := msg.String()
		switch {
		case s == "/":
			l.enterFilterMode()
			return l, textinput.Blink
		case s == "j", s == "down":
			l.moveCursor(+1)
			return l, nil
		case s == "k", s == "up":
			l.moveCursor(-1)
			return l, nil
		case msg.Type == tea.KeyEnter:
			return l, l.openCursor()
		case s == "s":
			return l, func() tea.Msg { return GoToSummaryMsg{} }
		case key.Matches(msg, l.keys.Accept):
			return l, l.setStatus(model.StatusAccepted)
		case key.Matches(msg, l.keys.Reject):
			return l, l.setStatus(model.StatusRejected)
		case key.Matches(msg, l.keys.Pending):
			return l, l.setStatus(model.StatusPending)
		}
	}
	return l, nil
}

// updateMouse handles wheel scrolling and row clicks. A click selects the
// row; a click on the already-selected row opens it (detail / summary) —
// so "double click" also works without tracking timing.
func (l *List) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		l.moveCursor(-1)
		return l, nil
	case msg.Button == tea.MouseButtonWheelDown:
		l.moveCursor(+1)
		return l, nil
	case !leftClick(msg):
		return l, nil
	}

	if hit(l.zones, ZoneListSummaryRow, msg) {
		if l.cursor == summaryCursor {
			return l, func() tea.Msg { return GoToSummaryMsg{} }
		}
		l.cursor = summaryCursor
		return l, nil
	}
	for pos := l.offset; pos < min(l.offset+l.rowsHeight, len(l.visibleIdx)); pos++ {
		if !hit(l.zones, fmt.Sprintf("%s%d", ZoneListRowPrefix, pos), msg) {
			continue
		}
		idx := l.visibleIdx[pos]
		if l.cursor == idx {
			return l, l.openCursor()
		}
		l.cursor = idx
		return l, nil
	}
	return l, nil
}

// openCursor opens whatever the cursor is on: the summary row or a comment.
func (l *List) openCursor() tea.Cmd {
	if l.cursor == summaryCursor {
		return func() tea.Msg { return GoToSummaryMsg{} }
	}
	idx := l.cursor
	return func() tea.Msg { return GoToDetailMsg{Index: idx} }
}

func (l *List) setStatus(st model.Status) tea.Cmd {
	c := l.selectedComment()
	if c == nil {
		return nil
	}
	c.Status = st
	// A status-based filter may hide the row we just touched.
	l.recomputeVisible()
	l.ensureCursorVisible()
	return dirty()
}

func (l *List) enterFilterMode() {
	l.filterMode = true
	l.filterInput.SetValue(l.filter.String())
	l.filterInput.CursorEnd()
	l.filterInput.Focus()
}

func (l *List) exitFilterMode() {
	l.filterMode = false
	l.filterInput.Blur()
}

func (l *List) updateFilterMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		l.exitFilterMode()
		return l, nil
	case tea.KeyEnter:
		expr := strings.TrimSpace(l.filterInput.Value())
		l.exitFilterMode()
		if expr == "" {
			l.ClearFilter()
			return l, nil
		}
		f, err := filter.Parse(expr)
		if err != nil {
			return l, func() tea.Msg { return FilterErrMsg{Err: err} }
		}
		l.SetFilter(f)
		return l, nil
	}
	var cmd tea.Cmd
	l.filterInput, cmd = l.filterInput.Update(msg)
	return l, cmd
}

func (l *List) moveCursor(delta int) {
	if l.cursor == summaryCursor {
		if delta > 0 && len(l.visibleIdx) > 0 {
			l.cursor = l.visibleIdx[0]
			l.ensureCursorVisible()
		}
		return
	}
	rowPos := indexOf(l.visibleIdx, l.cursor)
	if rowPos < 0 {
		l.cursor = summaryCursor
		return
	}
	next := rowPos + delta
	if next < 0 {
		l.cursor = summaryCursor
		return
	}
	if next >= len(l.visibleIdx) {
		return // clamp at last visible
	}
	l.cursor = l.visibleIdx[next]
	l.ensureCursorVisible()
}

// ensureCursorVisible scrolls the row viewport so the cursor's row is on
// screen. The offset is what mouse hit-testing keys off, so this is the
// single place it moves.
func (l *List) ensureCursorVisible() {
	pos := indexOf(l.visibleIdx, l.cursor)
	if pos < 0 {
		return
	}
	if pos < l.offset {
		l.offset = pos
	}
	if pos >= l.offset+l.rowsHeight {
		l.offset = pos - l.rowsHeight + 1
	}
	if maxOffset := max(0, len(l.visibleIdx)-l.rowsHeight); l.offset > maxOffset {
		l.offset = maxOffset
	}
}

func (l *List) View() string {
	parts := []string{l.headerView()}
	switch {
	case l.filterMode:
		parts = append(parts, l.filterInput.View())
	case !l.filter.IsEmpty():
		parts = append(parts, l.filterStatusView())
	}
	parts = append(parts, l.summaryRowView(), "", l.tableView(), l.footerView())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (l *List) headerView() string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	return style.Render(fmt.Sprintf("revu — %s #%d   head=%s   event=%s",
		l.review.PR.Repo, l.review.PR.Number, shortSHA(l.review.PR.HeadSHA), l.review.ReviewEvent))
}

func (l *List) filterStatusView() string {
	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("214")).
		Padding(0, 1)
	return style.Render(fmt.Sprintf("Filter: %s   (Esc to clear)", l.filter.String()))
}

func (l *List) summaryRowView() string {
	preview := summaryPreview(l.review.SummaryBody, 60)
	label := fmt.Sprintf("Summary  [%s]  %s", l.review.ReviewEvent, preview)

	marker := "  "
	style := lipgloss.NewStyle().Padding(0, 1)
	if l.cursor == summaryCursor {
		marker = "▶ "
		style = style.
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Bold(true)
	}
	return mark(l.zones, ZoneListSummaryRow, style.Render(marker+label))
}

// tableView renders the column header, a rule, and the visible row window.
// Each row is wrapped in a zone keyed by its visible position.
func (l *List) tableView() string {
	headStyle := lipgloss.NewStyle().Bold(true)
	rule := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	var cells []string
	for _, col := range listColumns {
		cells = append(cells, padCell(col.title, col.width))
	}
	header := headStyle.Render(" " + strings.Join(cells, " "))

	totalWidth := 1
	for _, col := range listColumns {
		totalWidth += col.width + 1
	}
	sep := rule.Render(strings.Repeat("─", totalWidth))

	selStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57"))

	rows := []string{header, sep}
	end := min(l.offset+l.rowsHeight, len(l.visibleIdx))
	for pos := l.offset; pos < end; pos++ {
		c := l.review.Comments[l.visibleIdx[pos]]
		line := " " + strings.Join([]string{
			padCell(c.ID, listColumns[0].width),
			padCell(string(c.Status), listColumns[1].width),
			padCell(string(c.Severity), listColumns[2].width),
			padCell(string(c.Category), listColumns[3].width),
			padCell(fmt.Sprintf("%s:%s", filepath.Base(c.Path), c.LineLabel()), listColumns[4].width),
		}, " ")
		if l.cursor == l.visibleIdx[pos] {
			line = selStyle.Render(line)
		}
		rows = append(rows, mark(l.zones, fmt.Sprintf("%s%d", ZoneListRowPrefix, pos), line))
	}
	// Pad so the footer stays put while the list shrinks below the window.
	for len(rows)-2 < l.rowsHeight {
		rows = append(rows, "")
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// padCell truncates or pads s to exactly width cells.
func padCell(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		if width <= 1 {
			return string(r[:width])
		}
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-len(r))
}

func (l *List) footerView() string {
	c := l.review.Counts()
	style := lipgloss.NewStyle().Faint(true).Padding(0, 1)
	visible := fmt.Sprintf("%d of %d", len(l.visibleIdx), len(l.review.Comments))
	if l.filter.IsEmpty() {
		return style.Render(fmt.Sprintf(
			"Pending: %d  Accepted: %d  Rejected: %d  Total: %d   click/[enter]open [s]ummary [/]filter [a]ccept [r]eject [u]ndo  [?]help",
			c[model.StatusPending], c[model.StatusAccepted], c[model.StatusRejected], len(l.review.Comments),
		))
	}
	return style.Render(fmt.Sprintf(
		"Showing: %s   Pending: %d  Accepted: %d  Rejected: %d   [/]filter [a]ccept [r]eject [u]ndo  [?]help",
		visible, c[model.StatusPending], c[model.StatusAccepted], c[model.StatusRejected],
	))
}

func (l *List) selectedComment() *model.Comment {
	if l.cursor < 0 || l.cursor >= len(l.review.Comments) {
		return nil
	}
	return &l.review.Comments[l.cursor]
}

// Cursor returns the current cursor index. -1 means the summary row.
func (l *List) Cursor() int { return l.cursor }

// VisibleCount returns how many comments pass the current filter.
func (l *List) VisibleCount() int { return len(l.visibleIdx) }

func (l *List) recomputeVisible() {
	l.visibleIdx = l.filter.VisibleIndices(l.review.Comments)
}

func dirty() tea.Cmd {
	return func() tea.Msg { return DirtyMsg{} }
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

func summaryPreview(body string, max int) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		runes := []rune(line)
		if len(runes) > max {
			return string(runes[:max-1]) + "…"
		}
		return line
	}
	return "(no summary)"
}

func indexOf(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
