// Package picker provides a small bubbletea list for choosing a PR from
// the "review-requested:@me" set. It runs to completion and returns the
// selected item (or nil when the user quits without selecting).
package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
	final, err := tea.NewProgram(m).Run()
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
}

func newModel(items []github.PRListItem) model {
	return model{items: items}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
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
		fmt.Fprintf(&b, "%s#%-5d %s\n", cursor, it.Number, title)
		meta := fmt.Sprintf("       %s → %s  by @%s", it.HeadRefName, it.BaseRefName, it.Author.Login)
		b.WriteString(dimStyle.Render(meta))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ or j/k: move   enter: select   q/esc: cancel"))
	b.WriteString("\n")
	return b.String()
}
