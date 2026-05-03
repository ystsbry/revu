package views

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// summaryCursor is the cursor value indicating the summary row is selected.
// Comment indices use 0..len(Comments)-1.
const summaryCursor = -1

// List is a tea.Model that renders the comment table with a summary selector
// row above it. Cursor -1 means the summary row is focused; any other value
// targets a row in the table.
type List struct {
	keys   keys.KeyMap
	table  table.Model
	review *model.Review
	cursor int
	width  int
	height int
}

// DirtyMsg is emitted when the in-memory Review has been mutated.
type DirtyMsg struct{}

// GoToDetailMsg requests the parent app to switch to the detail view at Index.
type GoToDetailMsg struct{ Index int }

// GoToSummaryMsg requests the parent app to switch to the summary view.
type GoToSummaryMsg struct{}

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

	l := &List{keys: km, table: t, review: r, cursor: summaryCursor}
	l.refreshRows()
	l.syncTableFocus()
	return l
}

func (l *List) Init() tea.Cmd { return nil }

func (l *List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height
		// Header (1) + summary row (1) + blank (1) + footer (1) + table border/header (3).
		h := msg.Height - 7
		if h < 5 {
			h = 5
		}
		l.table.SetHeight(h)
	case tea.KeyMsg:
		s := msg.String()
		switch {
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
	// Forward other messages (e.g. mouse, page up/down) to the table when it owns focus.
	if l.cursor >= 0 {
		var cmd tea.Cmd
		l.table, cmd = l.table.Update(msg)
		return l, cmd
	}
	return l, nil
}

func (l *List) moveCursor(delta int) {
	n := len(l.review.Comments)
	next := l.cursor + delta
	if next < summaryCursor {
		next = summaryCursor
	}
	if n == 0 {
		next = summaryCursor
	} else if next >= n {
		next = n - 1
	}
	l.cursor = next
	if next >= 0 {
		l.table.SetCursor(next)
	}
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
	header := l.headerView()
	summaryRow := l.summaryRowView()
	footer := l.footerView()
	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		summaryRow,
		"",
		l.table.View(),
		footer,
	)
}

func (l *List) headerView() string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	return style.Render(fmt.Sprintf("revu — %s #%d   head=%s   event=%s",
		l.review.PR.Repo, l.review.PR.Number, shortSHA(l.review.PR.HeadSHA), l.review.ReviewEvent))
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
	return style.Render(fmt.Sprintf(
		"Pending: %d  Accepted: %d  Rejected: %d  Total: %d   [enter]open [s]ummary [a]ccept [r]eject [u]ndo  [?]help",
		c[model.StatusPending], c[model.StatusAccepted], c[model.StatusRejected], len(l.review.Comments),
	))
}

// selectedComment returns the comment under the cursor, or nil when the
// summary row is selected.
func (l *List) selectedComment() *model.Comment {
	if l.cursor < 0 || l.cursor >= len(l.review.Comments) {
		return nil
	}
	return &l.review.Comments[l.cursor]
}

// Cursor returns the current cursor index. -1 means the summary row.
// Exposed for tests.
func (l *List) Cursor() int { return l.cursor }

func (l *List) refreshRows() {
	rows := make([]table.Row, 0, len(l.review.Comments))
	for _, c := range l.review.Comments {
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

// summaryPreview returns a short single-line preview of summary.md, stripping
// leading markdown heading markers ("#", "##", ...) from the first non-empty line.
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
