package render

import "github.com/charmbracelet/glamour"

// Markdown renders body to ANSI-styled output suitable for printing in a TUI
// pane of the given width. width <= 0 falls back to a sensible default.
func Markdown(body string, width int) (string, error) {
	if width <= 0 {
		width = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return "", err
	}
	defer func() { _ = r.Close() }()
	return r.Render(body)
}
