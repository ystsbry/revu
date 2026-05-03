package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// helpView is the static help overlay. It documents the same keys exposed
// in each view's footer plus the global commands.
func helpView(width, height int) string {
	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214")).
		Padding(0, 1).
		Render("revu — keybindings")

	sections := []struct {
		heading string
		rows    [][2]string
	}{
		{
			"Navigation",
			[][2]string{
				{"j / ↓", "next item / line down"},
				{"k / ↑", "prev item / line up"},
				{"Enter", "open detail (from list)"},
				{"s", "open summary (from list)"},
				{"n", "next comment (in detail)"},
				{"p", "prev comment (in detail)"},
				{"l", "back to list (from detail/summary)"},
			},
		},
		{
			"Status",
			[][2]string{
				{"a", "accept selected comment"},
				{"r", "reject selected comment"},
				{"u", "reset to pending"},
				{"c", "cycle review_event (summary)"},
			},
		},
		{
			"Files",
			[][2]string{
				{"e", "edit body in $EDITOR"},
				{"Ctrl+S", "save status changes"},
			},
		},
		{
			"Commands (after :)",
			[][2]string{
				{":save / :w", "persist status changes"},
				{":quit / :q", "quit (warns on unsaved)"},
				{":q!", "force quit, discarding changes"},
			},
		},
		{
			"Misc",
			[][2]string{
				{"?", "toggle this help"},
				{"q", "quit (warns on unsaved)"},
				{":", "command mode"},
				{"Esc", "leave command mode"},
			},
		},
	}

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)
	descStyle := lipgloss.NewStyle()
	headingStyle := lipgloss.NewStyle().Underline(true).Bold(true).Foreground(lipgloss.Color("33"))

	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, sec := range sections {
		b.WriteString(headingStyle.Render(sec.heading))
		b.WriteByte('\n')
		for _, kv := range sec.rows {
			b.WriteString("  ")
			b.WriteString(keyStyle.Render(padRight(kv[0], 12)))
			b.WriteString(descStyle.Render(kv[1]))
			b.WriteByte('\n')
		}
		b.WriteByte('\n')
	}
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("press ? or Esc to return"))

	innerW := width - 4
	if innerW < 30 {
		innerW = 30
	}
	innerH := height - 2
	if innerH < 10 {
		innerH = 10
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(innerW).
		Height(innerH).
		Render(b.String())
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
