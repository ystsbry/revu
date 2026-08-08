package picker

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

// LocalPRItem describes one reviewed pr-N/{sha} directory presented in the
// picker. ShortSHA is shown alongside the PR number so users can tell which
// commit a review was generated against.
type LocalPRItem struct {
	Number      int
	ShortSHA    string
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
	final, err := tea.NewProgram(m, tea.WithMouseCellMotion()).Run()
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
	zones  *zone.Manager
}

func newLocalModel(items []LocalPRItem) localModel {
	return localModel{items: items, zones: zone.New()}
}

func (m localModel) Init() tea.Cmd { return nil }

func (m localModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

// updateMouse mirrors the PR picker: wheel moves, click selects, click on
// the selected row confirms.
func (m localModel) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
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
		info := m.zones.Get(fmt.Sprintf("local:%d", i))
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
		sha := it.ShortSHA
		if sha == "" {
			sha = "-"
		}
		meta := fmt.Sprintf("%s · reviewed %s", sha, formatRelTime(it.GeneratedAt))
		row := fmt.Sprintf("%s%-7s %s", cursor, head, dimStyle.Render(meta))
		b.WriteString(m.zones.Mark(fmt.Sprintf("local:%d", i), row))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("click/↑/↓: move   click again/enter: select   q/esc: cancel"))
	b.WriteString("\n")
	return m.zones.Scan(b.String())
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
