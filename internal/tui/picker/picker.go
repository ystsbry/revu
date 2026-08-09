// Package picker provides a small bubbletea list for choosing a PR from
// the "review-requested:@me" set. It runs to completion and returns the
// selected item (or nil when the user quits without selecting).
package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"

	"github.com/ystsbry/revu/internal/github"
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
	dimStyle      = lipgloss.NewStyle().Faint(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("212"))
	helpStyle     = lipgloss.NewStyle().Faint(true)
)

// Pick runs the picker UI until the user selects a PR or quits.
// Returns nil when the user quits without selecting.
func Pick(items []github.PRListItem) (*github.PRListItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no PRs to pick from")
	}
	m := newModel(items)
	final, err := tea.NewProgram(m, tea.WithMouseCellMotion()).Run()
	if err != nil {
		return nil, err
	}
	out := final.(model)
	if !out.chose {
		return nil, nil
	}
	pick := out.items[out.cursor]
	return &pick, nil
}

type model struct {
	items  []github.PRListItem
	cursor int
	chose  bool
	width  int
	zones  *zone.Manager
}

func newModel(items []github.PRListItem) model {
	return model{items: items, zones: zone.New()}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case tea.MouseMsg:
		return m.updateMouse(msg)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			m.chose = true
			return m, tea.Quit
		case "g":
			m.cursor = 0
		case "G":
			m.cursor = len(m.items) - 1
		}
	}
	return m, nil
}

// updateMouse: wheel moves the cursor; clicking a row selects it, and a
// click on the already-selected row confirms — same contract as the
// review TUI's list.
func (m model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Button == tea.MouseButtonWheelUp:
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case msg.Button == tea.MouseButtonWheelDown:
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
		return m, nil
	case msg.Button != tea.MouseButtonLeft || msg.Action != tea.MouseActionPress:
		return m, nil
	}
	for i := range m.items {
		info := m.zones.Get(fmt.Sprintf("pick:%d", i))
		if info == nil || info.IsZero() || !info.InBounds(msg) {
			continue
		}
		if m.cursor == i {
			m.chose = true
			return m, tea.Quit
		}
		m.cursor = i
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Select a PR to review (%d awaiting)", len(m.items))))
	b.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		title := it.Title
		if i == m.cursor {
			cursor = cursorStyle.Render("▸ ")
			title = selectedStyle.Render(title)
		}
		row := fmt.Sprintf("%s#%-5d %s\n%s", cursor, it.Number, title,
			dimStyle.Render(fmt.Sprintf("       %s → %s  by @%s", it.HeadRefName, it.BaseRefName, it.Author.Login)))
		b.WriteString(m.zones.Mark(fmt.Sprintf("pick:%d", i), row))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("click/↑/↓: move   click again/enter: select   q/esc: cancel"))
	b.WriteString("\n")
	return m.zones.Scan(b.String())
}
