package picker

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// LocalPRItem describes one reviewed pr-N directory presented in the picker.
type LocalPRItem struct {
	Number      int
	Path        string
	GeneratedAt time.Time
}

// PickLocal runs a small list UI for choosing among already-reviewed PRs
// stored under ~/.revu/{owner}/{repo}/. Returns nil if the user quits.
func PickLocal(items []LocalPRItem) (*LocalPRItem, error) {
	if len(items) == 0 {
		return nil, fmt.Errorf("no reviewed PRs to pick from")
	}
	m := newLocalModel(items)
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, err
	}
	out := final.(localModel)
	if !out.chose {
		return nil, nil
	}
	pick := out.items[out.cursor]
	return &pick, nil
}

type localModel struct {
	items  []LocalPRItem
	cursor int
	chose  bool
	width  int
}

func newLocalModel(items []LocalPRItem) localModel {
	return localModel{items: items}
}

func (m localModel) Init() tea.Cmd { return nil }

func (m localModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m localModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Open a reviewed PR (%d)", len(m.items))))
	b.WriteString("\n\n")

	for i, it := range m.items {
		cursor := "  "
		head := fmt.Sprintf("#%d", it.Number)
		if i == m.cursor {
			cursor = cursorStyle.Render("▸ ")
			head = selectedStyle.Render(head)
		}
		meta := fmt.Sprintf("reviewed %s", formatRelTime(it.GeneratedAt))
		fmt.Fprintf(&b, "%s%-7s %s\n", cursor, head, dimStyle.Render(meta))
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ or j/k: move   enter: select   q/esc: cancel"))
	b.WriteString("\n")
	return b.String()
}

// formatRelTime renders t as a coarse "x ago" string. Falls back to the
// absolute timestamp when t is zero or in the future.
func formatRelTime(t time.Time) string {
	if t.IsZero() {
		return "(unknown time)"
	}
	d := time.Since(t)
	if d < 0 {
		return t.Format(time.RFC3339)
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}
