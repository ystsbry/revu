// Package dashboard hosts the multi-repository review dashboard opened by
// `revu tui`.
//
// The dashboard is a navigation shell: it owns a stack of screens (L0 repo
// list → L1 PR list → L2 PR actions → L3 the existing review TUI) and does
// nothing itself beyond routing messages to the active one, broadcasting
// resizes, and drawing the breadcrumb trail.
//
// This package deliberately knows nothing about repositories, PRs, or jobs;
// each screen is supplied by the workstream that owns it. The existing
// review TUI joins the stack through Embed, so internal/tui stays unaware
// it is being hosted.
package dashboard

import tea "github.com/charmbracelet/bubbletea"

// Screen is one entry on the navigation stack: a tea.Model that also knows
// its own breadcrumb label.
type Screen interface {
	tea.Model
	// Title is the short label shown in the breadcrumb trail.
	Title() string
}

// Embed adapts a plain tea.Model into a Screen with a fixed title.
//
// This is the seam the review TUI is pushed through:
//
//	dashboard.Push(dashboard.Embed("Review", tui.NewApp(cfg)))
//
// so a model written as a standalone bubbletea program can be hosted
// without being aware of the dashboard.
func Embed(title string, m tea.Model) Screen {
	return embedded{title: title, model: m}
}

type embedded struct {
	title string
	model tea.Model
}

func (e embedded) Title() string { return e.title }
func (e embedded) Init() tea.Cmd { return e.model.Init() }
func (e embedded) View() string  { return e.model.View() }

// Update forwards to the wrapped model and re-wraps whatever it returns,
// so the Screen identity (and its title) survives models that replace
// themselves on update.
func (e embedded) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := e.model.Update(msg)
	if next != nil {
		e.model = next
	}
	return e, cmd
}
