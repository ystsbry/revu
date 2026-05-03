package views

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/filter"
	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// summaryCursor indicates the summary row is selected. Any non-negative value
// is an index into review.Comments.
const summaryCursor = -1

// List renders the comment table with a summary selector row above it,
// optionally filtered by a filter.Filter expression.
type List struct {
	keys   keys.KeyMap
	table  table.Model
	review *model.Review
	cursor int

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
	cols := []table.Column{
		{Title: "#", Width: 4},
		{Title: "Status", Width: 9},
		{Title: "Sev", Width: 8},
		{Title: "Cat", Width: 8},
		{Title: "File:Line", Width: 50},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(false),
		table.WithHeight(10),
	)
	fi := textinput.New()
	fi.Prompt = "/"
	fi.CharLimit = 128
	fi.Placeholder = "severity:major,critical category:bug ..."

	l := &List{keys: km, table: t, review: r, cursor: summaryCursor, filterInput: fi}
	l.recomputeVisible()
	l.refreshRows()
	l.syncTableFocus()
	return l
}

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
		l.table.SetCursor(0)
	} else {
		l.cursor = summaryCursor
	}
	l.refreshRows()
	l.syncTableFocus()
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
		// Header (1) + filter line (0-1) + summary row (1) + blank (1) + footer (1) + table border/header (3).
		extra := 6
		if l.filterMode || !l.filter.IsEmpty() {
			extra++
		}
		h := msg.Height - extra
		if h < 5 {
			h = 5
		}
		l.table.SetHeight(h)
		l.filterInput.Width = msg.Width - 4
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
			if l.cursor == summaryCursor {
				return l, func() tea.Msg { return GoToSummaryMsg{} }
			}
			return l, func() tea.Msg { return GoToDetailMsg{Index: l.cursor} }
		case s == "s":
			return l, func() tea.Msg { return GoToSummaryMsg{} }
		case key.Matches(msg, l.keys.Accept):
			if c := l.selectedComment(); c != nil {
				c.Status = model.StatusAccepted
				l.refreshRows()
				return l, dirty()
			}
		case key.Matches(msg, l.keys.Reject):
			if c := l.selectedComment(); c != nil {
				c.Status = model.StatusRejected
				l.refreshRows()
				return l, dirty()
			}
		case key.Matches(msg, l.keys.Pending):
			if c := l.selectedComment(); c != nil {
				c.Status = model.StatusPending
				l.refreshRows()
				return l, dirty()
			}
		}
	}
	if l.cursor >= 0 {
		var cmd tea.Cmd
		l.table, cmd = l.table.Update(msg)
		return l, cmd
	}
	return l, nil
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
			l.table.SetCursor(0)
		}
		l.syncTableFocus()
		return
	}
	rowPos := indexOf(l.visibleIdx, l.cursor)
	if rowPos < 0 {
		l.cursor = summaryCursor
		l.syncTableFocus()
		return
	}
	next := rowPos + delta
	if next < 0 {
		l.cursor = summaryCursor
		l.syncTableFocus()
		return
	}
	if next >= len(l.visibleIdx) {
		return // clamp at last visible
	}
	l.cursor = l.visibleIdx[next]
	l.table.SetCursor(next)
	l.syncTableFocus()
}

func (l *List) syncTableFocus() {
	st := table.DefaultStyles()
	st.Header = st.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	if l.cursor == summaryCursor {
		l.table.Blur()
		st.Selected = lipgloss.NewStyle()
	} else {
		l.table.Focus()
		st.Selected = st.Selected.
			Foreground(lipgloss.Color("229")).
			Background(lipgloss.Color("57")).
			Bold(false)
	}
	l.table.SetStyles(st)
}

func (l *List) View() string {
	parts := []string{l.headerView()}
	switch {
	case l.filterMode:
		parts = append(parts, l.filterInput.View())
	case !l.filter.IsEmpty():
		parts = append(parts, l.filterStatusView())
	}
	parts = append(parts, l.summaryRowView(), "", l.table.View(), l.footerView())
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
	return style.Render(marker + label)
}

func (l *List) footerView() string {
	c := l.review.Counts()
	style := lipgloss.NewStyle().Faint(true).Padding(0, 1)
	visible := fmt.Sprintf("%d of %d", len(l.visibleIdx), len(l.review.Comments))
	if l.filter.IsEmpty() {
		return style.Render(fmt.Sprintf(
			"Pending: %d  Accepted: %d  Rejected: %d  Total: %d   [enter]open [s]ummary [/]filter [a]ccept [r]eject [u]ndo  [?]help",
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

func (l *List) refreshRows() {
	l.recomputeVisible()
	rows := make([]table.Row, 0, len(l.visibleIdx))
	for _, idx := range l.visibleIdx {
		c := l.review.Comments[idx]
		rows = append(rows, table.Row{
			c.ID,
			string(c.Status),
			string(c.Severity),
			string(c.Category),
			fmt.Sprintf("%s:%d", filepath.Base(c.Path), c.Line),
		})
	}
	l.table.SetRows(rows)
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
