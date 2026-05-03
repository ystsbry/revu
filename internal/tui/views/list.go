package views

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ystsbry/revu/internal/model"
	"github.com/ystsbry/revu/internal/tui/keys"
)

// List is a tea.Model that renders the comment table.
// It mutates Review.Comments[*].Status in place when accept/reject/pending
// keys are pressed, and signals the parent via DirtyMsg so the app shell
// can track unsaved changes.
type List struct {
	keys   keys.KeyMap
	table  table.Model
	review *model.Review
	width  int
	height int
}

// DirtyMsg is emitted when the in-memory Review has been mutated.
type DirtyMsg struct{}

func NewList(r *model.Review, keys keys.KeyMap) *List {
	cols := []table.Column{
		{Title: "#", Width: 4},
		{Title: "Status", Width: 9},
		{Title: "Sev", Width: 8},
		{Title: "Cat", Width: 8},
		{Title: "File:Line", Width: 50},
	}
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithHeight(10),
	)
	st := table.DefaultStyles()
	st.Header = st.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true)
	st.Selected = st.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)
	t.SetStyles(st)

	l := &List{keys: keys, table: t, review: r}
	l.refreshRows()
	return l
}

func (l *List) Init() tea.Cmd { return nil }

func (l *List) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height
		// Leave room for header (3 lines) and footer (3 lines).
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		l.table.SetHeight(h)
	case tea.KeyMsg:
		switch {
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
	var cmd tea.Cmd
	l.table, cmd = l.table.Update(msg)
	return l, cmd
}

func (l *List) View() string {
	header := l.headerView()
	footer := l.footerView()
	return lipgloss.JoinVertical(lipgloss.Left, header, l.table.View(), footer)
}

func (l *List) headerView() string {
	style := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	return style.Render(fmt.Sprintf("revu — %s #%d   head=%s   event=%s",
		l.review.PR.Repo, l.review.PR.Number, shortSHA(l.review.PR.HeadSHA), l.review.ReviewEvent))
}

func (l *List) footerView() string {
	c := l.review.Counts()
	style := lipgloss.NewStyle().Faint(true).Padding(0, 1)
	return style.Render(fmt.Sprintf(
		"Pending: %d  Accepted: %d  Rejected: %d  Total: %d   [a]ccept [r]eject [u]ndo  [:]cmd  [q]uit",
		c[model.StatusPending], c[model.StatusAccepted], c[model.StatusRejected], len(l.review.Comments),
	))
}

func (l *List) selectedComment() *model.Comment {
	idx := l.table.Cursor()
	if idx < 0 || idx >= len(l.review.Comments) {
		return nil
	}
	return &l.review.Comments[idx]
}

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
